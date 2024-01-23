/*
	(c) 2023 by Island Magic Co. All rights reserved.

	This program is a launcher for VARA modem programs. It can be used to start and stop
	the modem programs remotely. It also advertises the modem programs using zeroconf.
	The launcher can be run as a service on Windows and Linux.
	The launcher is configured using a JSON file. The default name of the configuration
	file is the same as the name of the executable with the extension changed to .json.
	The configuration file can be specified using the -config command line option.
	The launcher can be run as a service using the -service command line option.
*/

package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/grandcat/zeroconf"
)

// This gets set at build time derived from the git tag
// go build -ldflags "-X main.version=$(git describe --tags --always --dirty)"
var version = "undefined"

type Config struct {
	Port   int     `json:"Port"`
	Delay  int     `json:"Delay"`
	Modems []Modem `json:"Modems"`
}
type Modem struct {
	Name    string  `json:"Name"`
	Type    string  `json:"Type"`
	Cmd     string  `json:"Cmd"`
	Args    string  `json:"Args"`
	Config  string  `json:"Config"`
	CatCtrl CatCtrl `json:"CatCtrl,omitempty"`
	mu      sync.Mutex
}
type CatCtrl struct {
	Port    int    `json:"Port"`
	Dialect string `json:"Dialect"`
	Cmd     string `json:"Cmd"`
	Args    string `json:"Args"`
}
type program struct {
	ctx context.Context
	*Config
}

func assertExecutable(path string) error {
	_, err := exec.LookPath(path)
	if err != nil {
		return fmt.Errorf("Failed to find executable %q: %v", path, err)
	}
	return nil
}

func assertConfigFile(path string) error {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return fmt.Errorf("Failed to find config file %q: %v", path, err)
	}
	return nil
}

func (p *program) validateConfig() {
	// Iterate over modems and exit if no modem is defined
	if len(p.Modems) == 0 {
		log.Fatal("No modems defined")
	}

	// Iterate over modems and validate that all cmd map to an existing file
	for _, modem := range p.Modems {
		if modem.Cmd == "" {
			log.Fatalf("Modem executable for '%s' not defined", modem.Name)
		} else {
			err := assertExecutable(modem.Cmd)
			if err != nil {
				log.Fatal(err)
			}
		}

		if modem.Config != "" {
			err := assertConfigFile(modem.Config)
			if err != nil {
				log.Fatal(err)
			}
		}

		if modem.CatCtrl.Cmd != "" {
			err := assertExecutable(modem.CatCtrl.Cmd)
			if err != nil {
				log.Fatal(err)
			}
		}
	}
}

func addOption(options []string, key string, value string) []string {
	if value != "" {
		options = append(options, key+"="+value+";")
	}
	return options
}

func getConfigPath() (string, error) {
	fullexecpath, err := os.Executable()
	if err != nil {
		return "", err
	}

	dir, execname := filepath.Split(fullexecpath)
	ext := filepath.Ext(execname)
	name := execname[:len(execname)-len(ext)]

	return filepath.Join(dir, name+".json"), nil
}

func getConfig(path string) (*Config, error) {
	log.Println("Loading configuration from", path)
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	conf := &Config{}

	r := json.NewDecoder(f)
	err = r.Decode(&conf)
	if err != nil {
		return nil, err
	}
	return conf, nil
}

func findModem(modems []*Modem, name string) *Modem {
	for _, modem := range modems {
		if modem.Name == name {
			return modem
		}
	}
	return nil
}

func createCommand(multiWriter io.Writer, path string, args ...string) *exec.Cmd {
	fullPath, err := exec.LookPath(path)
	if err != nil {
		log.Println(err)
		return nil
	}
	cmd := exec.Command(fullPath, args...)
	cmd.Stdout = multiWriter
	cmd.Stderr = multiWriter
	cmd.Dir = filepath.Dir(fullPath)
	cmd.Env = os.Environ()
	return cmd
}

func defaultIniConfigPath(modem *Modem) (string, error) {
	// Figure out .ini file name for this modem
	iniFilePath, _ := DefaultVaraConfigFile(modem.Cmd)
	if !FileExists(iniFilePath) {
		// Try args for linux implementations
		iniFilePath, _ = DefaultVaraConfigFile(modem.Args)
		if !FileExists(iniFilePath) {
			log.Println("ERROR cannot find default .ini file for modem", modem.Name)
			return "", fmt.Errorf("cannot find default .ini file for modem %s", modem.Name)
		}
	}
	return iniFilePath, nil
}

func specifiedIniConfigPath(modem *Modem) (string, error) {
	iniFilePath := modem.Config
	if iniFilePath == "" {
		// If nothing specified, use default
		return defaultIniConfigPath(modem)
	}
	return iniFilePath, nil
}

func handleConnection(conn net.Conn, p *program) {
	var modemCmd *exec.Cmd
	var catCtrlCmd *exec.Cmd
	var configPath string

	dbfsLevels := make(chan DbfsLevel, 32)
	stop := make(chan bool)
	cmdChannel := make(chan string)

	var modem *Modem

	defer func() {
		log.Println("Cleaning up after closing connection")

		if modemCmd != nil && modemCmd.Process != nil {
			log.Println("Shutdown modem process gracefully")
			// Gracefully shutdown process on linux and kill on windows
			err := modemCmd.Process.Signal(syscall.SIGTERM)
			if err != nil {
				log.Println("Shutdown modem process gracefully failed, killing")
				modemCmd.Process.Kill()
			}
			processState, err := modemCmd.Process.Wait()
			if err != nil || processState.Success() {
				log.Println("Warning: awaiting termination of modem process failed")
			}
			modemCmd.Process.Release()
		}

		if configPath != "" {
			log.Println("Restoring original config file", configPath)
			os.Rename(configPath+".varanny.bak", configPath)
		}

		if catCtrlCmd != nil && catCtrlCmd.Process != nil {
			log.Println("Shutdown cat control process gracefully")
			// Gracefully shutdown process on linux and kill on windows
			err := catCtrlCmd.Process.Signal(syscall.SIGTERM)
			if err != nil {
				log.Println("Shutdown cat control process gracefully failed, killing")
				catCtrlCmd.Process.Kill()
			}
			processState, err := catCtrlCmd.Process.Wait()
			if err != nil || processState.Success() {
				log.Println("Warning: awaiting termination of modem process failed")
			}
			catCtrlCmd.Process.Release()
		}

		if modem != nil {
			// release mutex if still locked
			modem.mu.TryLock()
			modem.mu.Unlock()
		}

		conn.Close()

		stop <- true

		close(dbfsLevels)
		close(cmdChannel)
		close(stop)
	}()

	// Start a separate goroutine to read from a TCP socket
	go func() {
		buffer := make([]byte, 1024)
		for {
			n, err := conn.Read(buffer)
			if err != nil {
				if err == io.EOF {
					log.Println("Client closed the connection")
				} else {
					log.Println(err)
				}
				stop <- true
				return
			}
			cmdChannel <- strings.TrimSpace(string(buffer[:n]))
		}
	}()

	for {
		select {
		case dbfs := <-dbfsLevels:
			str := fmt.Sprintf("%.1f\n", dbfs.Level)
			conn.Write([]byte(str))
		case command := <-cmdChannel:
			log.Println("Received command:", command)
			if strings.Split(command, " ")[0] == "start" {
				// modem name could have spaces in it
				modemName := strings.TrimPrefix(command, "start ")
				modems := make([]*Modem, len(p.Modems))
				for i := range p.Modems {
					modems[i] = &p.Modems[i]
				}
				modem = findModem(modems, modemName)

				if modem != nil {
					if modem.mu.TryLock() == false {
						conn.Write([]byte("ERROR modem " + modemName + " is already running\n"))
						log.Println("ERROR modem " + modemName + " is already running")
						return
					}
					defer modem.mu.Unlock()

					var err error

					// Start cat control if defined first. No need to start VARA if cat control fails
					if modem.CatCtrl.Cmd != "" {
						logWriter := log.Writer()
						multiWriter := io.MultiWriter(logWriter)
						catCtrlCmd = createCommand(multiWriter, modem.CatCtrl.Cmd, strings.Split(modem.CatCtrl.Args, " ")...)

						if catCtrlCmd != nil {
							log.Println("Starting cat control for", modemName)
							log.Println("Command:", catCtrlCmd.Path, catCtrlCmd.Args)
							err = catCtrlCmd.Start()
						}
					}

					if err == nil && modem.Cmd != "" {
						logWriter := log.Writer()
						multiWriter := io.MultiWriter(logWriter)
						modemCmd = createCommand(multiWriter, modem.Cmd, modem.Args)

						if modemCmd != nil {

							// Swap the config file to the one defined in the modem if needed
							if modem.Config != "" {

								configPath, err = defaultIniConfigPath(modem)

								if err != nil {
									conn.Write([]byte("ERROR " + err.Error() + "\n"))
									return
								}
								modemConfigPath := modem.Config

								if modemConfigPath != configPath {
									// Check if requested modem config exists
									if !FileExists(modemConfigPath) {
										log.Println("Modem config file", modemConfigPath, "does not exist")
									} else {
										// Make backup
										log.Println("Backing up current config file", configPath)
										err := CopyFile(configPath, configPath+".varanny.bak")
										if err != nil {
											configPath = "" // prevent restore
											log.Println(err)
										} else {
											log.Println("Installing modem config file", modemConfigPath)
											err := CopyFile(modemConfigPath, configPath)
											if err != nil {
												log.Println(err)
											}
										}
									}
								}
							}

							log.Println("Starting modem for", modemName)
							log.Println("Command:", modemCmd.Path, modemCmd.Args)
							err = modemCmd.Start()
						}
					}

					if err != nil {
						conn.Write([]byte("ERROR\n"))
						log.Println(err)
						return
					} else {
						// Wait for modem to start and bind to their ports
						time.Sleep(3 * time.Second)
						conn.Write([]byte("OK\n"))
					}
				} else {
					conn.Write([]byte("ERROR modem name '" + modemName + "' not found\n"))
					return
				}
			} else {
				if strings.Split(command, " ")[0] == "monitor" {
					// modem name could have spaces in it
					modemName := strings.TrimPrefix(command, "monitor ")
					modems := make([]*Modem, len(p.Modems))
					for i := range p.Modems {
						modems[i] = &p.Modems[i]
					}
					modem = findModem(modems, modemName)

					if modem != nil {
						if modem.mu.TryLock() == false {
							conn.Write([]byte("ERROR modem " + modemName + " is already running\n"))
							log.Println("ERROR modem " + modemName + " is already running")
							return
						}
						defer modem.mu.Unlock()

						// Figure out .ini file name for this modem
						iniFilePath, err := specifiedIniConfigPath(modem)
						if err != nil {
							conn.Write([]byte("ERROR " + err.Error() + "\n"))
							return
						}

						// Lookup audio device name
						audioDeviceName, err := GetInputDeviceName(iniFilePath)
						if err != nil {
							conn.Write([]byte("ERROR audio device not found in " + iniFilePath + "\n"))
							return
						}

						log.Println("Monitoring audio device '" + audioDeviceName + "' found in " + iniFilePath)
						// start audio monitor
						device, err := FindAudioDevice(audioDeviceName)
						if err != nil {
							conn.Write([]byte("ERROR audio device '" + audioDeviceName + "' not found\n"))
							return
						}
						conn.Write([]byte("OK\n"))
						conn.Write([]byte(device.Name() + "\n"))
						go Monitor(device, dbfsLevels, stop)
					} else {
						conn.Write([]byte("ERROR modem name '" + modemName + "' not found\n"))
						return
					}
				} else {
					switch command {
					case "stop":
						conn.Write([]byte("OK\n"))
						return
					case "version":
						conn.Write([]byte("OK\n"))
						conn.Write([]byte(version + "\n"))
					case "list":
						conn.Write([]byte("OK\n"))
						for _, modem := range p.Modems {
							conn.Write([]byte(modem.Name + "\n"))
						}
					case "config":
						conn.Write([]byte("OK\n"))
						configPath, _ := getConfigPath()
						conn.Write([]byte("Config path: " + configPath + "\n"))
						for _, modem := range p.Modems {
							conn.Write([]byte(modem.Name + "\n"))
							conn.Write([]byte("  Type: " + modem.Type + "\n"))
							conn.Write([]byte("  Cmd: " + modem.Cmd + "\n"))
							conn.Write([]byte("  Args: " + modem.Args + "\n"))
							conn.Write([]byte("  Config: " + modem.Config + "\n"))
							conn.Write([]byte("  CatCtrl.Port: " + strconv.Itoa(modem.CatCtrl.Port) + "\n"))
							conn.Write([]byte("  CatCtrl.Dialect: " + modem.CatCtrl.Dialect + "\n"))
							conn.Write([]byte("  CatCtrl.Cmd: " + modem.CatCtrl.Cmd + "\n"))
							conn.Write([]byte("  CatCtrl.Args: " + modem.CatCtrl.Args + "\n"))
						}
					default:
						conn.Write([]byte("Invalid command\n"))
					}
				}
			}
		case <-stop:
			return
		case <-p.ctx.Done():
			return
		}
	}
}

// Returns array of zeroconf servers
func advertiseServices(modems []Modem, port int) (servers []*zeroconf.Server) {
	var name string

	log.Println("Advertising DNS-SD services")
	printMulticastInterfaces()

	for _, modem := range modems {
		if modem.Cmd != "" {
			options := []string{}

			options = addOption(options, "launchport", strconv.Itoa(port))

			if modem.CatCtrl.Port != 0 {
				options = addOption(options, "catport", strconv.Itoa(modem.CatCtrl.Port))
				options = addOption(options, "catdialect", modem.CatCtrl.Dialect)
			}

			// Advertise the modem based on its type
			switch modem.Type {
			case "fm":
				name = "_varafm-modem._tcp"
			case "hf":
				name = "_varahf-modem._tcp"
			default:
				log.Fatal("Unknown modem type: ", modem.Type)
			}

			// Figure out .ini file name for this modem
			iniFilePath, err := specifiedIniConfigPath(&modem)
			if err != nil {
				log.Fatal(err)
			}

			// Lookup port
			port, err := GetPort(iniFilePath)
			if err != nil {
				log.Fatal("ERROR port number not found in " + iniFilePath)
			}

			server, err := zeroconf.Register(modem.Name, name, "local.", port, options, nil)
			if err != nil {
				log.Fatal(err)
			}
			servers = append(servers, server)
		}
	}
	return servers
}

// Print out all the broadcast network interfaces
func printMulticastInterfaces() {
	ifaces, err := net.Interfaces()
	if err != nil {
		log.Fatal(err)
	}

	interfaces := []string{}
	for _, iface := range ifaces {
		if (iface.Flags & net.FlagUp) == 0 {
			continue // interface down
		}
		if (iface.Flags & net.FlagBroadcast) > 0 {
			interfaces = append(interfaces, iface.Name)
		}
	}
	log.Println("Multicast network interfaces:", interfaces)
}

func (p *program) run() {
	servers := advertiseServices(p.Modems, p.Port)
	defer func() {
		for _, server := range servers {
			server.Shutdown()
		}
	}()

	// Start the launcher server
	portStr := strconv.Itoa(p.Port)
	ln, err := net.Listen("tcp", ":"+portStr)
	if err != nil {
		log.Fatal(err)
	}
	log.Println("Listening on", ln.Addr())
	log.Println("Waiting for connections...")

	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				log.Fatal(err)
			}
			go func() {
				log.Println("New connection")
				handleConnection(conn, p)
			}()
		}
	}()

	select {
	case <-p.ctx.Done():
		ln.Close()
		return
	}
}

func main() {
	configFlag := flag.String("config", "", "Path to the configuration file.")
	versionFlag := flag.Bool("version", false, "Print version and exit.")

	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: %s [options]\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "Options:\n")
		flag.PrintDefaults()
	}

	flag.Parse()

	if *versionFlag {
		fmt.Println("varanny version " + version)
		return
	}

	configPath := ""
	if len(*configFlag) != 0 {
		configPath = *configFlag
	} else {
		var err error
		configPath, err = getConfigPath()
		if err != nil {
			log.Fatal(err)
		}
	}

	config, err := getConfig(configPath)
	if err != nil {
		log.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	prg := &program{
		ctx:    ctx,
		Config: config,
	}

	prg.validateConfig()

	// Intercept SIGINT and SIGTERM to allow for graceful shutdown
	go func() {
		c := make(chan os.Signal, 1)
		signal.Notify(c, syscall.SIGINT, syscall.SIGTERM)
		<-c
		log.Println("Shutting down...")
		cancel()
	}()

	log.Println("Starting varanny", version)

	// Delay start of service to allow time for hotspot network to come up
	// Provide a default valut for Delay if not defined in config
	if config.Delay == 0 {
		config.Delay = 10
	}
	time.Sleep(time.Duration(config.Delay) * time.Second)

	prg.run()
}

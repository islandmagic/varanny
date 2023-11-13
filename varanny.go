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
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/grandcat/zeroconf"
	"github.com/kardianos/service"
)

var version = "0.1.14"

type Config struct {
	Port   int     `json:"Port"`
	Delay  int     `json:"Delay,default=10"`
	Modems []Modem `json:"Modems"`
}
type Modem struct {
	Name    string  `json:"Name"`
	Type    string  `json:"Type"`
	Cmd     string  `json:"Cmd"`
	Args    string  `json:"Args"`
	Config  string  `json:"Config"`
	Port    int     `json:"Port"`
	CatCtrl CatCtrl `json:"CatCtrl,omitempty"`
}
type CatCtrl struct {
	Port    int    `json:"Port"`
	Dialect string `json:"Dialect"`
	Cmd     string `json:"Cmd"`
	Args    string `json:"Args"`
}

var logger service.Logger

type program struct {
	exit    chan struct{}
	service service.Service

	*Config
}

func assertExecutable(path string) error {
	_, err := exec.LookPath(path)
	if err != nil {
		return fmt.Errorf("Failed to find executable %q: %v", path, err)
	}
	return nil
}

func (p *program) Start(s service.Service) error {
	// Iterate over modems and exit if no modem is defined
	if len(p.Modems) == 0 {
		logger.Info("No modems defined")
		return nil
	}

	// Iterate over modems and validate that all cmd map to an existing file
	for _, modem := range p.Modems {
		if modem.Cmd != "" {
			err := assertExecutable(modem.Cmd)
			if err != nil {
				return err
			}
		}
		if modem.CatCtrl.Cmd != "" {
			err := assertExecutable(modem.CatCtrl.Cmd)
			if err != nil {
				return err
			}
		}
	}

	go p.run()
	return nil
}

func addOption(options []string, key string, value string) []string {
	if value != "" {
		options = append(options, key+"="+value+";")
	}
	return options
}

func (p *program) run() {
	logger.Info("Starting launcher, listening on port ", p.Port)

	defer func() {
		if service.Interactive() {
			p.Stop(p.service)
		} else {
			p.service.Stop()
		}
	}()

	// Iterate over modem and announce them
	for _, modem := range p.Modems {
		if modem.Port != 0 {
			options := []string{}

			// Advertise the launcher port if the modem has a command
			if modem.Cmd != "" || modem.CatCtrl.Cmd != "" {
				options = addOption(options, "launchport", strconv.Itoa(p.Port))
			}

			if modem.CatCtrl.Port != 0 {
				options = addOption(options, "catport", strconv.Itoa(modem.CatCtrl.Port))
				options = addOption(options, "catdialect", modem.CatCtrl.Dialect)
			}

			// Advertise the modem based on its type
			switch modem.Type {
			case "fm":
				fm_server, err := zeroconf.Register(modem.Name, "_varafm-modem._tcp", "local.", modem.Port, options, nil)
				if err != nil {
					log.Fatal(err)
				}
				defer fm_server.Shutdown()
			case "hf":
				hf_server, err := zeroconf.Register(modem.Name, "_varahf-modem._tcp", "local.", modem.Port, options, nil)
				if err != nil {
					log.Fatal(err)
				}
				defer hf_server.Shutdown()
			default:
				log.Fatal("Unknown modem type: ", modem.Type)
			}
		}
	}

	// Start the launcher server
	portStr := strconv.Itoa(p.Port)
	ln, err := net.Listen("tcp", ":"+portStr)
	if err != nil {
		log.Fatal(err)
	}

	for {
		conn, err := ln.Accept()
		if err != nil {
			log.Fatal(err)
		}
		go func() {
			handleConnection(conn, p)
		}()
	}
}

func (p *program) Stop(s service.Service) error {
	close(p.exit)
	logger.Info("Stopping launcher")
	if service.Interactive() {
		os.Exit(0)
	}
	return nil
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

func findModem(modems []Modem, name string) *Modem {
	for _, modem := range modems {
		if modem.Name == name {
			return &modem
		}
	}
	return nil
}

// Create a command with the given path and arguments
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

func handleConnection(conn net.Conn, p *program) {
	var modemCmd *exec.Cmd
	var catCtrlCmd *exec.Cmd
	var configPath string
	var modemConfigPath string
	dbfsLevels := make(chan DbfsLevel, 32)
	stop := make(chan bool)
	cmdChannel := make(chan string)

	defer func() {
		log.Println("Closing connection")

		if modemCmd != nil && modemCmd.Process != nil {
			log.Println("Shutdown modem process gracefully")
			// Gracefully shutdown process on linux and kill on windows
			err := modemCmd.Process.Signal(syscall.SIGTERM)
			if err != nil {
				log.Println("Shutdown modem process gracefully failed, killing")
				modemCmd.Process.Kill()
			}
			modemCmd.Process.Release()
		}

		if configPath != "" {
			if modemConfigPath != "" {
				log.Println("Uninstalling modem config file", modemConfigPath)
				os.Rename(configPath, modemConfigPath)
			}
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
			catCtrlCmd.Process.Release()
		}

		stop <- true
		conn.Close()
	}()

	// Start a separate goroutine to read from a TCP socket
	go func() {
		buffer := make([]byte, 1024)
		for {
			n, err := conn.Read(buffer)
			if err != nil {
				if err == io.EOF {
					fmt.Println("Client closed the connection")
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
				modem := findModem(p.Modems, modemName)

				if modem != nil {
					var err error

					// Star cat control if defined first. No need to start VARA if cat control fails
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

							// Swap the config file to the one defined in the modem
							if modem.Config != "" {
								// Rename existing config file
								// Extract path from modem Cmd to find existing config file
								if modem.Type == "fm" {
									configPath = filepath.Join(filepath.Dir(modem.Config), "VARAFM.ini")
								} else {
									configPath = filepath.Join(filepath.Dir(modem.Config), "VARA.ini")
								}
								modemConfigPath = modem.Config
								// Make backup
								log.Println("Backing up current config file", configPath)
								err := os.Rename(configPath, configPath+".varanny.bak")
								if err != nil {
									configPath = "" // prevent restore
									log.Println(err)
								} else {
									// Swap config file
									log.Println("Installing modem config file", modemConfigPath)
									err := os.Rename(modemConfigPath, configPath)
									if err != nil {
										modemConfigPath = "" // prevent restore
										log.Println(err)
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
					modem := findModem(p.Modems, modemName)

					if modem != nil {
						// Figure out .ini file name for this modem
						iniFilePath := modem.Config
						if iniFilePath == "" {
							// Use default .ini file name
							iniFilePath = DefaultVaraConfigFile(modem.Cmd)
						}
						// Lookup audio device name
						audioDeviceName, err := GetInputDeviceName(iniFilePath)
						if err != nil {
							conn.Write([]byte("ERROR audio device not found in " + iniFilePath + "\n"))
							return
						}
						log.Println("Monitoring audio device", audioDeviceName)
						// start audio monitor
						device, err := FindAudioDevice(audioDeviceName)
						if err != nil {
							conn.Write([]byte("ERROR audio device '" + audioDeviceName + "' not found\n"))
							return
						}
						conn.Write([]byte("OK\n"))
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
							conn.Write([]byte("  Port: " + strconv.Itoa(modem.Port) + "\n"))
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
		}
	}
}

func main() {
	svcFlag := flag.String("service", "", "Control the system service.")
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

	svcConfig := &service.Config{
		Name:        "varanny",
		DisplayName: "VARA Modem Nanny",
		Description: "This service can start and stop vara modem program remotely.",
	}

	prg := &program{
		exit: make(chan struct{}),

		Config: config,
	}
	s, err := service.New(prg, svcConfig)
	if err != nil {
		log.Fatal(err)
	}
	prg.service = s

	errs := make(chan error, 5)
	logger, err = s.Logger(errs)
	if err != nil {
		log.Fatal(err)
	}

	go func() {
		for {
			err := <-errs
			if err != nil {
				log.Print(err)
			}
		}
	}()

	if len(*svcFlag) != 0 {
		err := service.Control(s, *svcFlag)
		if err != nil {
			log.Printf("Valid actions: %q\n", service.ControlAction)
			log.Fatal(err)
		}
		return
	}

	// Delay start of service to allow time for hotspot network to come up
	time.Sleep(time.Duration(config.Delay) * time.Second)

	err = s.Run()
	if err != nil {
		logger.Error(err)
	}
}

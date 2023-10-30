package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/grandcat/zeroconf"
	"github.com/kardianos/service"
)

var version = "0.1.2"

type Config struct {
	Port   int     `json:"Port"`
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

// Create a command with the given path and arguments
func createCommand(path string, args ...string) *exec.Cmd {
	fullPath, err := exec.LookPath(path)
	if err != nil {
		log.Println(err)
		return nil
	}
	cmd := exec.Command(fullPath, args...)
	cmd.Stdout = nil
	cmd.Stderr = nil
	cmd.Dir = filepath.Dir(fullPath)
	cmd.Env = os.Environ()
	return cmd
}

func handleConnection(conn net.Conn, p *program) {
	buffer := make([]byte, 1024)
	var modemCmd *exec.Cmd
	var catCtrlCmd *exec.Cmd
	var configPath string
	var modemConfigPath string

	defer func() {
		if modemCmd != nil && modemCmd.Process != nil {
			modemCmd.Process.Kill()
		}
		if catCtrlCmd != nil && catCtrlCmd.Process != nil {
			catCtrlCmd.Process.Kill()
		}
		if configPath != "" {
			if modemConfigPath != "" {
				os.Rename(configPath, modemConfigPath)
			}
			os.Rename(configPath+".varanny.bak", configPath)
		}
		conn.Close()
	}()

	for {
		n, err := conn.Read(buffer)
		if err != nil {
			log.Println(err)
			conn.Close()
			return
		}

		command := strings.TrimSpace(string(buffer[:n]))
		if strings.Split(command, " ")[0] == "start" {
			// modem name could have spaces in it
			modemName := strings.TrimPrefix(command, "start ")
			found := false
			for _, modem := range p.Modems {
				if modem.Name == modemName {
					found = true
					var err error
					if modem.Cmd != "" {
						modemCmd = createCommand(modem.Cmd, strings.Split(modem.Args, " ")...)
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
								err := os.Rename(configPath, configPath+".varanny.bak")
								if err != nil {
									log.Println(err)
								} else {
									// Swap config file
									err := os.Rename(modemConfigPath, configPath)
									if err != nil {
										log.Println(err)
									}
								}
							}
							err = modemCmd.Start()
						}
					}

					if err == nil && modem.CatCtrl.Cmd != "" {
						catCtrlCmd = createCommand(modem.CatCtrl.Cmd, strings.Split(modem.CatCtrl.Args, " ")...)
						if catCtrlCmd != nil {
							err = catCtrlCmd.Start()
						}
					}

					if err != nil {
						conn.Write([]byte("ERROR\n"))
						conn.Close()
						log.Println(err)
					} else {
						conn.Write([]byte("OK\n"))
					}
					break
				}
			}
			if !found {
				conn.Write([]byte("ERROR Modem name '" + modemName + "' not found\n"))
			}
		} else {
			switch command {
			case "stop":
				conn.Write([]byte("OK\n"))
				conn.Close()
				return
			case "version":
				conn.Write([]byte(version + "\n"))
			case "exit":
				conn.Close()
				return
			default:
				conn.Write([]byte("Invalid command\n"))
			}
		}
	}
}

func main() {
	svcFlag := flag.String("service", "", "Control the system service.")
	configFlag := flag.String("config", "", "Path to the configuration file.")
	flag.Parse()

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

	err = s.Run()
	if err != nil {
		logger.Error(err)
	}
}

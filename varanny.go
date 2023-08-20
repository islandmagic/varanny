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

type Config struct {
	Name string
	Port int

	ExecFM string
	ExecHF string
	PortFM int
	PortHF int
}

var logger service.Logger

type program struct {
	exit    chan struct{}
	service service.Service

	*Config

	cmdfm *exec.Cmd
	cmdhf *exec.Cmd
}

func (p *program) Start(s service.Service) error {
	// Look for exec
	fmExec, err := exec.LookPath(p.ExecFM)
	if err != nil {
		return fmt.Errorf("Failed to find executable %q: %v", p.ExecFM, err)
	}

	hfExec, err := exec.LookPath(p.ExecHF)
	if err != nil {
		return fmt.Errorf("Failed to find executable %q: %v", p.ExecHF, err)
	}

	p.cmdfm = createCommand(fmExec)
	p.cmdfm.Dir = filepath.Dir(fmExec)
	p.cmdfm.Env = os.Environ()

	p.cmdhf = createCommand(hfExec)
	p.cmdhf.Dir = filepath.Dir(hfExec)
	p.cmdhf.Env = os.Environ()

	go p.run()
	return nil
}

func (p *program) run() {
	logger.Info("Starting ", p.Name)

	defer func() {
		if service.Interactive() {
			p.Stop(p.service)
		} else {
			p.service.Stop()
		}
	}()

	fmPortStr := strconv.Itoa(p.PortFM)
	hfPortStr := strconv.Itoa(p.PortHF)

	server, err := zeroconf.Register(p.Name, "_vara-modem._tcp", "local.", p.Port, []string{"fm=" + fmPortStr, "hf=" + hfPortStr}, nil)
	if err != nil {
		log.Fatal(err)
	}
	defer server.Shutdown()

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
		go handleConnection(conn, p.cmdfm, p.cmdhf)
	}
}

func (p *program) Stop(s service.Service) error {
	close(p.exit)
	logger.Info("Stopping ", p.Name)
	if p.cmdfm.Process != nil {
		p.cmdfm.Process.Kill()
	}
	if p.cmdhf.Process != nil {
		p.cmdhf.Process.Kill()
	}
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

func main() {
	svcFlag := flag.String("service", "", "Control the system service.")
	flag.Parse()

	configPath, err := getConfigPath()
	if err != nil {
		log.Fatal(err)
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

func createCommand(path string) *exec.Cmd {
	cmd := exec.Command(path)
	cmd.Stdout = nil
	cmd.Stderr = nil
	return cmd
}

func handleConnection(conn net.Conn, cmdVaraFM *exec.Cmd, cmdVaraHF *exec.Cmd) {
	buffer := make([]byte, 1024)

	n, err := conn.Read(buffer)
	if err != nil {
		log.Println(err)
	}

	command := strings.TrimSpace(string(buffer[:n]))
	switch command {
	case "START VARAFM":
		if cmdVaraFM.Process != nil {
			cmdVaraFM.Process.Kill()
		}
		err := cmdVaraFM.Start()
		if err != nil {
			log.Fatal(err)
			conn.Write([]byte("ERROR\n"))
		} else {
			conn.Write([]byte("OK\n"))
		}

	case "START VARAHF":
		if cmdVaraHF.Process != nil {
			cmdVaraHF.Process.Kill()
		}
		err := cmdVaraHF.Start()
		if err != nil {
			log.Fatal(err)
			conn.Write([]byte("ERROR\n"))
		} else {
			conn.Write([]byte("OK\n"))
		}

	case "STOP VARAFM":
		if cmdVaraFM.Process != nil {
			cmdVaraFM.Process.Kill()
		}
		conn.Write([]byte("OK\n"))

	case "STOP VARAHF":
		if cmdVaraHF.Process != nil {
			cmdVaraHF.Process.Kill()
		}
		conn.Write([]byte("OK\n"))

	case "VERSION":
		conn.Write([]byte("1.0\n"))

	default:
		conn.Write([]byte("Invalid command\n"))
	}

	conn.Close()
}

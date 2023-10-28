package main

import (
	"encoding/json"
	"flag"
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

var version = "0.0.14"

type Config struct {
	Port   int  `json:"Port"`
	VaraFM Exec `json:"VaraFM,omitempty"`
	VaraHF Exec `json:"VaraHF,omitempty"`
}
type Exec struct {
	Name    string  `json:"Name"`
	Cmd     string  `json:"Cmd"`
	Args    string  `json:"Args"`
	Port    int     `json:"Port"`
	CatCtrl CatCtrl `json:"CatCtrl,omitempty"`
}
type CatCtrl struct {
	Port    int    `json:"Port"`
	Dialect string `json:"Dialect"`
}

var logger service.Logger

type program struct {
	exit    chan struct{}
	service service.Service

	*Config

	cmdfm *exec.Cmd
	cmdhf *exec.Cmd
}

func createCommand(path string, args ...string) *exec.Cmd {
	cmd := exec.Command(path, args...)
	cmd.Stdout = nil
	cmd.Stderr = nil
	return cmd
}

func (p *program) Start(s service.Service) error {
	// Look for exec
	if p.VaraFM.Cmd != "" {
		fmExec, err := exec.LookPath(p.VaraFM.Cmd)
		if err != nil {
			//return fmt.Errorf("Failed to find executable %q: %v", p.VaraFM.Cmd, err)
		}

		p.cmdfm = createCommand(fmExec, p.VaraFM.Args)
		p.cmdfm.Dir = filepath.Dir(fmExec)
		p.cmdfm.Env = os.Environ()
	} else {
		logger.Info("No VARA FM executable defined")
	}

	if p.VaraHF.Cmd != "" {
		hfExec, err := exec.LookPath(p.VaraHF.Cmd)
		if err != nil {
			//return fmt.Errorf("Failed to find executable %q: %v", p.VaraHF.Cmd, err)
		}

		p.cmdhf = createCommand(hfExec, p.VaraHF.Args)
		p.cmdhf.Dir = filepath.Dir(hfExec)
		p.cmdhf.Env = os.Environ()
	} else {
		logger.Info("No VARA HF executable defined")
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

	if p.VaraFM.Port != 0 {
		options := []string{}

		if p.cmdfm != nil {
			options = addOption(options, "launchport", strconv.Itoa(p.Port))
		}
		if p.VaraFM.CatCtrl.Port != 0 {
			options = addOption(options, "catport", strconv.Itoa(p.VaraFM.CatCtrl.Port))
			options = addOption(options, "catdialect", p.VaraFM.CatCtrl.Dialect)
		}
		fm_server, err := zeroconf.Register(p.VaraFM.Name, "_varafm-modem._tcp", "local.", p.VaraFM.Port, options, nil)
		if err != nil {
			log.Fatal(err)
		}
		defer fm_server.Shutdown()
	}

	if p.VaraHF.Port != 0 {
		options := []string{}

		if p.cmdhf != nil {
			options = addOption(options, "launchport", strconv.Itoa(p.Port))
		}
		if p.VaraHF.CatCtrl.Port != 0 {
			options = addOption(options, "catport", strconv.Itoa(p.VaraHF.CatCtrl.Port))
			options = addOption(options, "catdialect", p.VaraHF.CatCtrl.Dialect)
		}

		hf_server, err := zeroconf.Register(p.VaraHF.Name, "_varahf-modem._tcp", "local.", p.VaraHF.Port, options, nil)
		if err != nil {
			log.Fatal(err)
		}
		defer hf_server.Shutdown()
	}

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
			cmdfm := createCommand(p.cmdfm.Path, p.cmdfm.Args...)
			cmdfm.Dir = p.cmdfm.Dir
			cmdfm.Env = os.Environ()

			cmdhf := createCommand(p.cmdhf.Path, p.cmdhf.Args...)
			cmdhf.Dir = p.cmdhf.Dir
			cmdhf.Env = os.Environ()

			handleConnection(conn, cmdfm, cmdhf)
		}()
	}
}

func (p *program) Stop(s service.Service) error {
	close(p.exit)
	logger.Info("Stopping laucnher")
	if p.cmdfm != nil && p.cmdfm.Process != nil {
		p.cmdfm.Process.Kill()
	}
	if p.cmdhf != nil && p.cmdhf.Process != nil {
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

func handleConnection(conn net.Conn, cmdVaraFM *exec.Cmd, cmdVaraHF *exec.Cmd) {
	buffer := make([]byte, 1024)

	for {
		n, err := conn.Read(buffer)
		if err != nil {
			log.Println(err)
			conn.Close()
			return
		}

		command := strings.TrimSpace(string(buffer[:n]))
		switch command {
		case "START VARAFM":
			startCommand(cmdVaraFM, conn)
		case "START VARAHF":
			startCommand(cmdVaraHF, conn)
		case "STOP VARAFM":
			stopCommand(cmdVaraFM, conn)
		case "STOP VARAHF":
			stopCommand(cmdVaraHF, conn)
		case "VERSION":
			conn.Write([]byte(version + "\n"))
		case "EXIT":
			conn.Close()
			return
		default:
			conn.Write([]byte("Invalid command\n"))
		}
	}
}

func startCommand(cmd *exec.Cmd, conn net.Conn) {
	if cmd != nil && cmd.Process != nil {
		stopCommand(cmd, conn)
	}
	err := cmd.Start()
	if err != nil {
		conn.Write([]byte("ERROR\n"))
		log.Fatal(err)
	} else {
		conn.Write([]byte("OK\n"))
	}
}

func stopCommand(cmd *exec.Cmd, conn net.Conn) {
	if cmd != nil && cmd.Process != nil {
		err := cmd.Process.Kill()
		if err != nil {
			log.Fatal(err)
		}
	}
	conn.Write([]byte("OK\n"))
}

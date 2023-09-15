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

var version = "0.0.8"

type Config struct {
	Name   string `json:"Name"`
	Port   int    `json:"Port"`
	VaraFM Exec   `json:"VaraFM,omitempty"`
	VaraHF Exec   `json:"VaraHF,omitempty"`
}
type Exec struct {
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
			return fmt.Errorf("Failed to find executable %q: %v", p.VaraFM.Cmd, err)
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
			return fmt.Errorf("Failed to find executable %q: %v", p.VaraHF.Cmd, err)
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

func (p *program) run() {
	logger.Info("Starting ", p.Name, " listening on port ", p.Port)

	defer func() {
		if service.Interactive() {
			p.Stop(p.service)
		} else {
			p.service.Stop()
		}
	}()

	var options []string

	fmPortStr := strconv.Itoa(p.VaraFM.Port)

	// Include VaraFM attributes only if they are not empty
	if fmPortStr != "" {
		options = append(options, "fm="+fmPortStr)
	}

	fmCatPortStr := strconv.Itoa(p.VaraFM.CatCtrl.Port)
	fmCatDialectStr := p.VaraFM.CatCtrl.Dialect

	if fmCatPortStr != "" {
		options = append(options, "fmcat="+fmCatPortStr)
	}
	if fmCatDialectStr != "" {
		options = append(options, "fmcatd="+fmCatDialectStr)
	}

	hfPortStr := strconv.Itoa(p.VaraHF.Port)

	// Include VaraHF attributes only if they are not empty
	if hfPortStr != "" {
		options = append(options, "hf="+hfPortStr)
	}

	hfCatPortStr := strconv.Itoa(p.VaraHF.CatCtrl.Port)
	hfCatDialectStr := p.VaraHF.CatCtrl.Dialect

	if hfCatPortStr != "" {
		options = append(options, "hfcat="+hfCatPortStr)
	}
	if hfCatDialectStr != "" {
		options = append(options, "hfcatd="+hfCatDialectStr)
	}

	server, err := zeroconf.Register(p.Name, "_vara-modem._tcp", "local.", p.Port, options, nil)

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
	default:
		conn.Write([]byte("Invalid command\n"))
	}

	conn.Close()
}

func startCommand(cmd *exec.Cmd, conn net.Conn) {
	if cmd != nil && cmd.Process != nil {
		cmd.Process.Kill()
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
		cmd.Process.Kill()
	}
	conn.Write([]byte("OK\n"))
}

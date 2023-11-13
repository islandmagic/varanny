package main

import (
	"fmt"
	"path/filepath"

	"github.com/go-ini/ini"
)

/*
Parse the .ini file and return a map of the key/value pairs

[Soundcard]
Input Device Name=Microphone (USB Audio CODEC )
Output Device Name=Speakers (USB Audio CODEC )
*/
func GetInputDeviceName(path string) (string, error) {
	inidata, err := ini.Load(path)
	if err != nil {
		fmt.Printf("Fail to read file: %v", err)
		return "", err
	}
	section := inidata.Section("Soundcard")
	return section.Key("Input Device Name").String(), err
}

func DefaultVaraConfigFile(fullexecpath string) string {
	dir, execname := filepath.Split(fullexecpath)
	switch execname {
	case "VARA.exe":
		return dir + "/VARA.ini"
	case "VARAFM.exe":
		return dir + "/VARAFM.ini"
	default:
		return ""
	}
}

package main

import (
	"fmt"
	"io"
	"os"
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
		return filepath.Join(dir, "VARA.ini")
	case "VARAFM.exe":
		return filepath.Join(dir, "VARAFM.ini")
	default:
		return ""
	}
}

// Check if the file exists
func FileExists(filename string) bool {
	if _, err := os.Stat(filename); err == nil {
		return true
	}
	return false
}

// Copy the file from the source to the destination
func CopyFile(source string, destination string) error {
	// Open the source file
	src, err := os.Open(source)
	if err != nil {
		return err
	}
	defer src.Close()

	// Create the destination file
	dst, err := os.Create(destination)
	if err != nil {
		return err
	}
	defer dst.Close()

	// Copy the bytes from source to destination
	_, err = io.Copy(dst, src)
	if err != nil {
		return err
	}
	return err
}

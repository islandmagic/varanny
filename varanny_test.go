package main

import (
	"os"
	"os/exec"
	"path"
	"testing"
)

func TestAssertExecutableError(t *testing.T) {
	path := "this-command-does-not-exist"
	err := assertExecutable(path)

	if err == nil {
		t.Errorf("Expected %s to be not executable, but got %s", path, err)
	}
}
func TestAssertExecutable(t *testing.T) {
	path := "echo" // Use a standard command that exists on most systems
	err := assertExecutable(path)

	if err != nil {
		t.Errorf("Expected %s to be executable, but got %s", path, err)
	}
}
func TestGetConfigPath(t *testing.T) {
	configPath, err := getConfigPath()

	if err != nil {
		t.Fatalf("Cannot find config %s", err)
	}

	if configPath == "" {
		t.Error("Expected config path to be not empty")
	}
}
func TestGetConfig(t *testing.T) {
	config, err := getConfig(path.Join("sample-configs", "varanny.test.json"))

	if err != nil {
		t.Fatalf("Cannot find config %s", err)
	}

	if config == nil {
		t.Error("Expected config to be not nil")
	}
}
func TestCreateCommand(t *testing.T) {
	path := "echo" // Use a standard command that exists on most systems
	args := []string{"arg1", "arg2"}

	cmd := createCommand(nil, path, args...)

	expectedPath, err := exec.LookPath(path)
	if err != nil {
		t.Fatalf("Cannot find command %s in PATH: %v", path, err)
	}

	if cmd.Path != expectedPath {
		t.Errorf("Expected command path to be %s, but got %s", expectedPath, cmd.Path)
	}

	if len(cmd.Args) != len(args)+1 {
		t.Errorf("Expected command arguments length to be %d, but got %d", len(args)+1, len(cmd.Args))
	}

	for i, arg := range args {
		if cmd.Args[i+1] != arg {
			t.Errorf("Expected argument at index %d to be %s, but got %s", i+1, arg, cmd.Args[i+1])
		}
	}
}

func TestMain(m *testing.M) {
	// Do setup here
	code := m.Run()
	// Do teardown here
	os.Exit(code)
}

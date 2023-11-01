package main

import (
	"os/exec"
	"testing"
)

func TestCreateCommand(t *testing.T) {
	path := "echo" // Use a standard command that exists on most systems
	args := []string{"arg1", "arg2"}

	cmd := createCommand(path, args...)

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

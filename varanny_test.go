package main

import (
	"testing"
)

func TestCreateCommand(t *testing.T) {
	path := "/path/to/command"
	args := []string{"arg1", "arg2"}

	cmd := createCommand(path, args...)

	if cmd.Path != path {
		t.Errorf("Expected command path to be %s, but got %s", path, cmd.Path)
	}

	if len(cmd.Args) != len(args)+1 {
		t.Errorf("Expected command arguments length to be %d, but got %d", len(args)+1, len(cmd.Args))
	}

	for i, arg := range args {
		if cmd.Args[i+1] != arg {
			t.Errorf("Expected argument at index %d to be %s, but got %s", i+1, arg, cmd.Args[i+1])
		}
	}

	if cmd.Stdout != nil {
		t.Error("Expected command stdout to be nil")
	}

	if cmd.Stderr != nil {
		t.Error("Expected command stderr to be nil")
	}
}

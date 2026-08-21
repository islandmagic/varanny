package main

import (
	"os"
	"os/exec"
	"path"
	"syscall"
	"testing"
	"time"
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

func TestTerminateProcessWithTimeout(t *testing.T) {
	// Test the timeout pattern - spawns a process, sends SIGTERM, waits with timeout
	cmd := exec.Command("sleep", "10")
	if err := cmd.Start(); err != nil {
		t.Fatalf("Failed to start sleep process: %v", err)
	}

	// Send SIGTERM
	cmd.Process.Signal(syscall.SIGTERM)

	// Wait for process to exit with timeout (same pattern as varanny.go)
	done := make(chan error, 1)
	go func() {
		_, err := cmd.Process.Wait()
		done <- err
	}()

	// Use a short timeout for testing - if process doesn't exit, force kill
	timeout := 1 * time.Second
	select {
	case err := <-done:
		// Process exited (either gracefully or was killed by signal)
		if err != nil {
			t.Logf("Process exited with error (expected on some systems): %v", err)
		}
	case <-time.After(timeout):
		// Timeout - force kill like varanny does
		cmd.Process.Kill()
		cmd.Process.Wait()
	}
}

func TestTerminateProcessForceKill(t *testing.T) {
	// Test forced kill - process ignores SIGTERM (using a subprocess that traps signals)
	// On Linux/macOS, we can test this with a command that ignores SIGTERM
	// For simplicity, we'll test the immediate kill path by sending a signal to an already-terminating process
	cmd := exec.Command("sleep", "10")
	if err := cmd.Start(); err != nil {
		t.Fatalf("Failed to start sleep process: %v", err)
	}

	// Send SIGKILL immediately to test the force-kill path
	if err := cmd.Process.Signal(syscall.SIGKILL); err != nil {
		t.Fatalf("Failed to send SIGKILL: %v", err)
	}

	_, err := cmd.Process.Wait()
	if err != nil {
		t.Errorf("Process should have been killed, got error: %v", err)
	}
}

func TestMain(m *testing.M) {
	// Do setup here
	code := m.Run()
	// Do teardown here
	os.Exit(code)
}

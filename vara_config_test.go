package main

import (
	"path/filepath"
	"testing"
)

func TestDefaultVaraConfigFileOverride(t *testing.T) {
	got, err := DefaultVaraConfigFile("/somewhere/VARA.exe", "/custom/VARA.ini")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "/custom/VARA.ini" {
		t.Fatalf("expected override path %q, got %q", "/custom/VARA.ini", got)
	}
}

func TestDefaultVaraConfigFileInferenceVara(t *testing.T) {
	dir := "/somewhere"
	got, err := DefaultVaraConfigFile(filepath.Join(dir, "VARA.exe"), "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := filepath.Join(dir, "VARA.ini")
	if got != want {
		t.Fatalf("expected %q, got %q", want, got)
	}
}

func TestDefaultVaraConfigFileInferenceVarafm(t *testing.T) {
	dir := "/somewhere"
	got, err := DefaultVaraConfigFile(filepath.Join(dir, "VARAFM.exe"), "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := filepath.Join(dir, "VARAFM.ini")
	if got != want {
		t.Fatalf("expected %q, got %q", want, got)
	}
}

func TestDefaultVaraConfigFileUnknownExe(t *testing.T) {
	_, err := DefaultVaraConfigFile("/somewhere/UNKNOWN.exe", "")
	if err == nil {
		t.Fatalf("expected error for unknown executable name")
	}
}


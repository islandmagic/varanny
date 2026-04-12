//go:build !linux

package main

import "testing"

func TestHostPowerOffUnsupported(t *testing.T) {
	err := hostPowerOff()
	if err == nil {
		t.Fatal("expected error on non-Linux")
	}
}

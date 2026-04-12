//go:build !linux

package main

import "errors"

func hostPowerOff() error {
	return errors.New("remote shutdown is only supported on Linux")
}

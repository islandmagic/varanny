//go:build linux

package main

import "golang.org/x/sys/unix"

func hostPowerOff() error {
	return unix.Reboot(unix.LINUX_REBOOT_CMD_POWER_OFF)
}

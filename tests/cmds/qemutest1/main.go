// Package qemutest1 is just a test program for qemu/ tests.
package main

import (
	"fmt"
	"log"

	"golang.org/x/sys/unix"
)

func main() {
	fmt.Println("I AM HERE")

	if err := unix.Reboot(unix.LINUX_REBOOT_CMD_POWER_OFF); err != nil {
		log.Fatalf("Failed to shut down: %v", err)
	}
}

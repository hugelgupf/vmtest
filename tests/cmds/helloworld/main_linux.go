// Package helloworld prints hello world and shuts down.
package main

import (
	"fmt"
	"log"

	"golang.org/x/sys/unix"
)

func main() {
	fmt.Println("Hello world")

	if err := unix.Reboot(unix.LINUX_REBOOT_CMD_POWER_OFF); err != nil {
		log.Fatalf("Failed to shut down: %v", err)
	}
}

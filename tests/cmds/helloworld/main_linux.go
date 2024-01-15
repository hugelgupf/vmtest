// Package helloworld prints hello world and shuts down.
package main

import (
	"flag"
	"fmt"
	"log"

	"golang.org/x/sys/unix"
)

var n = flag.Int("n", 1, "How many times to repeat Hello world")

func main() {
	flag.Parse()

	for i := 0; i < *n; i++ {
		fmt.Println("Hello world", i)
	}

	if err := unix.Reboot(unix.LINUX_REBOOT_CMD_POWER_OFF); err != nil {
		log.Fatalf("Failed to shut down: %v", err)
	}
}

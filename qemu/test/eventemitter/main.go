package main

import (
	"flag"
	"log"
	"strings"

	"github.com/hugelgupf/vmtest/guest"
	"github.com/hugelgupf/vmtest/qemu/test/eventemitter/event"
	"golang.org/x/sys/unix"
)

var skipClose = flag.Bool("skip-close", false, "Skip closing event channel")

func Main() {
	f, err := guest.SerialEventChannel[event.Event]("test")
	if err != nil {
		log.Fatal(err)
	}
	if !*skipClose {
		defer f.Close()
	}

	for i := 0; i < 1000; i++ {
		// Emit an ID with some variable length string.
		// Variable length string would mess up a PTY once larger than
		// window size.
		if err := f.Emit(event.Event{ID: i, String: strings.Repeat("a", i)}); err != nil {
			log.Fatal(err)
		}
	}
}

func main() {
	flag.Parse()
	Main()

	log.Println("TEST PASSED")

	if err := unix.Reboot(unix.LINUX_REBOOT_CMD_POWER_OFF); err != nil {
		log.Fatalf("Failed to shut down: %v", err)
	}
}

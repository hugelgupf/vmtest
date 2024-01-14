// Command httpdownload configures networking with DHCP and downloads one web
// page.
package main

import (
	"flag"
	"fmt"
	"log"
	"os"

	"github.com/hugelgupf/vmtest/guest"
	"golang.org/x/sys/unix"
)

var file = flag.String("file", "", "Filepath to read")

func realMain() error {
	cleanup, err := guest.MountSharedDir()
	if err != nil {
		return err
	}
	defer cleanup()

	b, err := os.ReadFile(*file)
	if err != nil {
		return err
	}

	fmt.Println(string(b))
	return nil
}

func main() {
	flag.Parse()

	if err := realMain(); err != nil {
		// Don't Fatalf, so that we can shutdown properly below.
		log.Printf("helloworld: %v", err)
	}

	_ = unix.Reboot(unix.LINUX_REBOOT_CMD_POWER_OFF)
}

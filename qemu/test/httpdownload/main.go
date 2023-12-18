// Command httpdownload configures networking with DHCP and downloads one web
// page.
package main

import (
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"

	"golang.org/x/sys/unix"
)

var (
	url = flag.String("url", "", "URL to download")
)

func realMain() error {
	cmd := exec.Command("dhclient", "-ipv6=false", "-vv")
	cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("dhclient: %v", err)
	}

	r, err := http.Get(*url)
	if err != nil {
		return fmt.Errorf("could not download HTTP: %v", err)
	}
	c, err := io.ReadAll(r.Body)
	if err != nil {
		return fmt.Errorf("could not read HTTP body: %v", err)
	}
	if r.StatusCode != 200 {
		return fmt.Errorf("%s due to %s", r.Status, string(c))
	}

	fmt.Printf("here's %s:\n", *url)
	fmt.Println(string(c))
	return nil
}

func main() {
	flag.Parse()

	if err := realMain(); err != nil {
		// Don't Fatalf, so that we can shutdown properly below.
		log.Printf("httpdownload: %v", err)
	}

	_ = unix.Reboot(unix.LINUX_REBOOT_CMD_POWER_OFF)
}

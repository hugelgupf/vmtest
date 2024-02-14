// Copyright 2021 the u-root Authors. All rights reserved
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Command shareduinit is a shared uinit between Go and shell tests.
//
// It also allows setting up GOCOVERDIR before calling the real uinit, which
// means we can collect test coverage from those uinits.
package main

import (
	"log"
	"os"
	"os/exec"

	"github.com/hugelgupf/vmtest/guest"
	"golang.org/x/sys/unix"
)

func run() error {
	covCleanup := guest.GOCOVERDIR()
	defer covCleanup()

	cleanup, err := guest.MountSharedDir()
	if err != nil {
		return err
	}
	defer cleanup()

	c := exec.Command("/bin/vminit", os.Args[1:]...)
	c.Stdout, c.Stderr = os.Stdout, os.Stderr
	return c.Run()
}

func main() {
	if err := run(); err != nil {
		log.Printf("Failed: %v", err)
	}

	if err := unix.Reboot(unix.LINUX_REBOOT_CMD_POWER_OFF); err != nil {
		log.Fatalf("Failed to reboot: %v", err)
	}
}

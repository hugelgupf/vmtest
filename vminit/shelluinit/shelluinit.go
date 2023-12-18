// Copyright 2021 the u-root Authors. All rights reserved
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Command shelluinit runs commands from an elvish script.
package main

import (
	"fmt"
	"log"
	"os"
	"os/exec"

	"github.com/hugelgupf/vmtest/guest"
	"github.com/hugelgupf/vmtest/vminit/common"
	"golang.org/x/sys/unix"
)

func runTest() error {
	cleanup, err := common.MountSharedDir()
	if err != nil {
		return err
	}
	defer cleanup()
	defer guest.CollectKernelCoverage()

	// Run the test script test.elv
	var test string
	for _, script := range []string{"/test.elv", "/testdata/test.elv"} {
		if _, err := os.Stat(script); err == nil {
			test = script
			continue
		} else if !os.IsNotExist(err) {
			return err
		}
	}
	if test == "" {
		return fmt.Errorf("could not find test script")
	}
	cmd := exec.Command("elvish", test)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("test.elv ran unsuccessfully: %v", err)
	}
	return nil
}

func main() {
	if err := runTest(); err != nil {
		log.Printf("Tests failed: %v", err)
	} else {
		log.Print("TESTS PASSED MARKER")
	}

	if err := unix.Reboot(unix.LINUX_REBOOT_CMD_POWER_OFF); err != nil {
		log.Fatalf("Failed to reboot: %v", err)
	}
}

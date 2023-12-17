// Copyright 2023 the u-root Authors. All rights reserved
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package vmtest can run commands or Go tests in VM guests for testing.
//
// TODO: say more.
package vmtest

import (
	"fmt"
	"os"
	"testing"

	"github.com/hugelgupf/vmtest/qemu"
)

// VMOptions are QEMU VM integration test options.
type VMOptions struct {
	// Name is the test's name.
	//
	// If name is left empty, t.Name() will be used.
	Name string

	// GuestArch is a setup function that sets the architecture.
	//
	// The default is qemu.ArchUseEnvv, which will use VMTEST_ARCH.
	GuestArch qemu.Arch

	// QEMUOpts are options to the QEMU VM.
	QEMUOpts []qemu.Fn

	// SharedDir is a directory shared with the QEMU VM using 9P.
	//
	// If none is set, no directory is shared with the guest.
	SharedDir string
}

// StartVM fills in some default options if not already provided, and starts a VM.
//
// StartVM uses a caller-supplied QEMU binary, architecture, kernel and
// initramfs, or fills them in from VMTEST_QEMU, VMTEST_QEMU_ARCH,
// VMTEST_KERNEL and VMTEST_INITRAMFS environment variables as is documented by
// the qemu package.
func StartVM(t testing.TB, o *VMOptions) *qemu.VM {
	SkipWithoutQEMU(t)

	// This is used by the console output logger in every t.Logf line to
	// prefix console statements.
	var consoleOutputName string
	if len(o.Name) == 0 {
		o.Name = t.Name()
		// Unnamed VMs likely means there's only 1 VM in the test. No
		// need to take up screen width with the test name.
		consoleOutputName = "serial"
	} else {
		// If the caller named this test, it's likely they are starting
		// more than 1 VM in the same test. Distinguish serial output
		// by putting the name of the VM in every console log line.
		consoleOutputName = fmt.Sprintf("%s serial", o.Name)
	}

	qopts := []qemu.Fn{
		qemu.LogSerialByLine(qemu.PrintLineWithPrefix(consoleOutputName, t.Logf)),
		// Tests use this cmdline arg to identify they are running inside a
		// vmtest using SkipIfNotInVM
		qemu.WithAppendKernel("uroot.vmtest"),
		qemu.VirtioRandom(),
	}
	if o.SharedDir != "" {
		qopts = append(qopts, qemu.P9Directory(o.SharedDir, false, ""))
	}

	// Prepend our default options so user-supplied o.QEMUOpts supersede.
	vm, err := qemu.Start(o.GuestArch, append(qopts, o.QEMUOpts...)...)
	if err != nil {
		t.Fatalf("Failed to start QEMU VM %s: %v", o.Name, err)
	}

	t.Cleanup(func() {
		t.Logf("QEMU command line to reproduce %s:\n%s", o.Name, vm.CmdlineQuoted())
	})
	return vm
}

// SkipWithoutQEMU skips the test when the QEMU environment variable is not
// set.
func SkipWithoutQEMU(t testing.TB) {
	if _, ok := os.LookupEnv("VMTEST_QEMU"); !ok {
		t.Skip("QEMU vmtest is skipped unless VMTEST_QEMU is set")
	}
}

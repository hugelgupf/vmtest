// Copyright 2022 the u-root Authors. All rights reserved
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package vmtest

import (
	"flag"
	"fmt"
	"os"
	"testing"

	"github.com/hugelgupf/vmtest/qemu"
)

var (
	keepSharedDir = flag.Bool("keep-shared-dir", false, "Keep shared directory after test, even if test passed")
)

// VMOptions are QEMU VM integration test options.
type VMOptions struct {
	// Name is the test's name.
	//
	// If name is left empty, t.Name() will be used.
	Name string

	// GuestArch is a setup function that sets the architecture.
	//
	// The default is qemu.GuestArchUseEnvv, which will use VMTEST_QEMU_ARCH.
	GuestArch qemu.ArchFn

	// QEMUOpts are options to the QEMU VM.
	QEMUOpts []qemu.Fn

	// SharedDir is the temporary directory exposed to the QEMU VM.
	//
	// If none is set, t.TempDir will be used.
	SharedDir string
}

// StartVM fills in some default options if not already provided, and starts a VM.
//
// StartVM uses a caller-supplied kernel and initramfs, or fills them in from
// VMTEST_KERNEL and VMTEST_INITRAMFS environment variables.
//
//   - TODO: overhaul timouts.
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

	if o.SharedDir == "" {
		o.SharedDir = t.TempDir()
	}
	arch := o.GuestArch
	if arch == nil {
		arch = qemu.GuestArchUseEnvv
	}

	var qopts []qemu.Fn
	switch arch.Arch() {
	case qemu.GuestArchX8664:
		qopts = append(qopts, qemu.WithAppendKernel("console=ttyS0 earlyprintk=ttyS0"))
	case qemu.GuestArchArm:
		qopts = append(qopts, qemu.WithAppendKernel("console=ttyAMA0"))
	}

	qopts = append(qopts,
		qemu.LogSerialByLine(qemu.PrintLineWithPrefix(consoleOutputName, t.Logf)),
		// Tests use this cmdline arg to identify they are running inside a
		// vmtest using SkipIfNotInVM
		qemu.WithAppendKernel("uroot.vmtest"),
		qemu.VirtioRandom,
		qemu.WithDevice(qemu.P9Directory{Dir: o.SharedDir}),
	)

	// Prepend our default options so user-supplied o.QEMUOpts supersede.
	vm, err := qemu.Start(arch, append(qopts, o.QEMUOpts...)...)
	if err != nil {
		t.Fatalf("Failed to start QEMU VM %s: %v", o.Name, err)
	}

	t.Cleanup(func() {
		t.Logf("QEMU command line to reproduce %s:\n%s", o.Name, vm.CmdlineQuoted())
		if t.Failed() {
			t.Log("Keeping temp dir: ", o.SharedDir)
		} else if !*keepSharedDir {
			if err := os.RemoveAll(o.SharedDir); err != nil {
				t.Logf("failed to remove temporary directory %s: %v", o.SharedDir, err)
			}
		}

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

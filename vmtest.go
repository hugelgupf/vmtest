// Copyright 2022 the u-root Authors. All rights reserved
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package vmtest

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/hugelgupf/vmtest/qemu"
	"github.com/u-root/u-root/pkg/cmdline"
	"github.com/u-root/u-root/pkg/cp"
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

	// QEMUOpts are QEMU VM options for the test.
	//
	// This is where kernel, initramfs, and devices are set up.
	//
	// If the kernel is not set, VMTEST_KERNEL will be used.
	// If the initramfs is not set, VMTEST_INITRAMFS will be used.
	// Default serial console output to t.Logf is set up in StartVM.
	QEMUOpts qemu.Options

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

	if o.QEMUOpts.SerialOutput == nil {
		o.QEMUOpts.SerialOutput = TestLineWriter(t, consoleOutputName)
	}
	if len(o.QEMUOpts.Kernel) == 0 {
		// Copy kernel to o.SharedDir for tests involving kexec.
		kernel := filepath.Join(o.SharedDir, "kernel")
		if err := cp.Copy(os.Getenv("VMTEST_KERNEL"), kernel); err != nil {
			t.Fatalf("Could not copy VMTEST_KERNEL: %v", err)
		}
		o.QEMUOpts.Kernel = kernel
	}
	if len(o.QEMUOpts.Initramfs) == 0 {
		// Copy initramfs to o.SharedDir.
		initramfs := filepath.Join(o.SharedDir, "initramfs.cpio")
		if err := cp.Copy(os.Getenv("VMTEST_INITRAMFS"), initramfs); err != nil {
			t.Fatalf("Could not copy VMTEST_INITRAMFS: %v", err)
		}
		o.QEMUOpts.Initramfs = initramfs
	}

	// Make sure console gets to the logs.
	arch, err := o.QEMUOpts.Arch()
	if err != nil {
		t.Fatal(err)
	}
	switch arch {
	case "x86_64":
		o.QEMUOpts.KernelArgs += " console=ttyS0 earlyprintk=ttyS0"
	case "arm":
		o.QEMUOpts.KernelArgs += " console=ttyAMA0"
	}

	// Tests use this cmdline arg to identify they are running inside a
	// vmtest using SkipIfNotInVM
	o.QEMUOpts.KernelArgs += " uroot.vmtest"

	o.QEMUOpts.Devices = append(o.QEMUOpts.Devices,
		qemu.VirtioRandom{},
		qemu.P9Directory{Dir: o.SharedDir},
	)

	vm, err := o.QEMUOpts.Start()
	if err != nil {
		t.Fatalf("Failed to start QEMU VM %s: %v", o.Name, err)
	}

	t.Cleanup(func() {
		vm.Close()

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

// SkipIfNotInVM skips the test if it is not running in a vmtest-started VM.
//
// The presence of "uroot.vmtest" on the kernel commandline is used to
// determine this.
func SkipIfNotInVM(t testing.TB) {
	if !cmdline.ContainsFlag("uroot.vmtest") {
		t.Skip("Skipping test -- must be run inside vmtest VM")
	}
}

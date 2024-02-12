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
	"github.com/hugelgupf/vmtest/uqemu"
	"github.com/u-root/mkuimage/uimage"
	"golang.org/x/exp/slices"
)

// VMOptions are QEMU VM integration test options.
type VMOptions struct {
	// Name is the test's name.
	//
	// If name is left empty, t.Name() will be used.
	Name                string
	ConsoleOutputPrefix string

	// GuestArch is a setup function that sets the architecture.
	//
	// The default is qemu.ArchUseEnvv, which will use VMTEST_ARCH.
	GuestArch qemu.Arch

	// QEMUOpts are options to the QEMU VM.
	QEMUOpts []qemu.Fn

	// SharedDir is a directory shared with the QEMU VM using 9P using the
	// tag "tmpdir".
	//
	// guest.MountSharedDir mounts this directory at /testdata.
	//
	// If none is set, no directory is shared with the guest by default.
	SharedDir string

	// Initramfs is an optional u-root initramfs to build.
	Initramfs []uimage.Modifier
}

// Opt is used to configure a VM.
type Opt func(testing.TB, *VMOptions) error

// WithName is the name of the VM, used for the serial console log output prefix.
func WithName(name string) Opt {
	return func(_ testing.TB, v *VMOptions) error {
		v.Name = name
		// If the caller named this test, it's likely they are starting
		// more than 1 VM in the same test. Distinguish serial output
		// by putting the name of the VM in every console log line.
		v.ConsoleOutputPrefix = fmt.Sprintf("%s vm", name)
		return nil
	}
}

// WithArch sets the guest architecture.
func WithArch(arch qemu.Arch) Opt {
	return func(_ testing.TB, v *VMOptions) error {
		v.GuestArch = arch
		return nil
	}
}

// WithQEMUFn adds QEMU options.
func WithQEMUFn(fn ...qemu.Fn) Opt {
	return func(_ testing.TB, v *VMOptions) error {
		v.QEMUOpts = append(v.QEMUOpts, fn...)
		return nil
	}
}

// WithUimage merges o with already appended initramfs build options.
func WithUimage(mods ...uimage.Modifier) Opt {
	return func(_ testing.TB, v *VMOptions) error {
		v.Initramfs = append(v.Initramfs, mods...)
		return nil
	}
}

// WithBusyboxCommands merges more busybox commands into the initramfs build options.
//
// Note that busybox rewrites commands, so if attempting to get integration
// test coverage of commands, use WithBinaryCommands.
func WithBusyboxCommands(cmds ...string) Opt {
	return WithUimage(uimage.WithBusyboxCommands(cmds...))
}

// WithBinaryCommands merges more binary commands into the initramfs build options.
func WithBinaryCommands(cmds ...string) Opt {
	return WithUimage(uimage.WithBinaryCommands(cmds...))
}

// WithInitramfsFiles merges more extra files into the initramfs build options.
// Syntax is like u-root's ExtraFiles.
func WithInitramfsFiles(files ...string) Opt {
	return WithUimage(uimage.WithFiles(files...))
}

// WithSharedDir shares a directory with the QEMU VM using 9P using the
// tag "tmpdir".
//
// guest.MountSharedDir mounts this directory at /testdata.
//
// If none is set, no directory is shared with the guest by default.
func WithSharedDir(dir string) Opt {
	return func(_ testing.TB, v *VMOptions) error {
		v.SharedDir = dir
		return nil
	}
}

// StartVM fills in some default options if not already provided, and starts a VM.
//
// StartVM uses a caller-supplied QEMU binary, architecture, kernel and
// initramfs, or fills them in from VMTEST_QEMU, VMTEST_QEMU_ARCH,
// VMTEST_KERNEL and VMTEST_INITRAMFS environment variables as is documented by
// the qemu package.
//
// By default, StartVM adds command-line streaming to t.Logf, appends
// VMTEST_IN_GUEST=1 to the kernel command-line.
//
// StartVM will print the QEMU command-line for reproduction when the test
// finishes. The test will fail if VM.Wait is not called.
func StartVM(t testing.TB, opts ...Opt) *qemu.VM {
	o := &VMOptions{
		Name: t.Name(),
		// Unnamed VMs likely means there's only 1 VM in the test. No
		// need to take up screen width with the test name.
		ConsoleOutputPrefix: "vm",
	}

	for _, opt := range opts {
		if opt != nil {
			if err := opt(t, o); err != nil {
				t.Fatal(err)
			}
		}
	}
	return startVM(t, o)
}

func startVM(t testing.TB, o *VMOptions) *qemu.VM {
	SkipWithoutQEMU(t)

	qopts := []qemu.Fn{
		// Tests use this env var to identify they are running inside a
		// vmtest using SkipIfNotInVM.
		qemu.WithAppendKernel("VMTEST_IN_GUEST=1"),
	}
	if o.SharedDir != "" {
		qopts = append(qopts,
			qemu.P9Directory(o.SharedDir, "tmpdir"),
			qemu.WithAppendKernel("VMTEST_SHARED_DIR=tmpdir"),
		)
	}
	if len(o.Initramfs) > 0 {
		qopts = append(qopts, uqemu.WithUimageT(t, o.Initramfs...))
	}

	// Prepend our default options so user-supplied o.QEMUOpts supersede.
	return qemu.StartT(t, o.Name, o.GuestArch, append(qopts, o.QEMUOpts...)...)
}

// SkipWithoutQEMU skips the test when the QEMU environment variable is not
// set.
func SkipWithoutQEMU(t testing.TB) {
	if _, ok := os.LookupEnv("VMTEST_QEMU"); !ok {
		t.Skip("QEMU vmtest is skipped unless VMTEST_QEMU is set")
	}
}

// SkipIfNotArch skips this test if VMTEST_ARCH is not one of the given values.
func SkipIfNotArch(t testing.TB, allowed ...qemu.Arch) {
	if arch := qemu.GuestArch(); !slices.Contains(allowed, arch) {
		t.Skipf("Skipping test because arch is %s, not in allowed set %v", arch, allowed)
	}
}

// Copyright 2023 the u-root Authors. All rights reserved
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package vmtest can run commands or Go tests in VM guests for testing.
//
// TODO: say more.
package vmtest

import (
	"fmt"
	"testing"

	"github.com/hugelgupf/vmtest/qemu"
	"github.com/hugelgupf/vmtest/qemu/quimage"
	"github.com/u-root/mkuimage/uimage"
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
	qemu.SkipWithoutQEMU(t)

	qopts := []qemu.Fn{
		// Tests use this env var to identify they are running inside a
		// vmtest using SkipIfNotInVM.
		qemu.WithAppendKernel("VMTEST_IN_GUEST=1"),
	}
	if len(o.Initramfs) > 0 {
		qopts = append(qopts, quimage.WithUimageT(t, o.Initramfs...))
	}

	// Prepend our default options so user-supplied o.QEMUOpts supersede.
	return qemu.StartT(t, o.Name, o.GuestArch, append(qopts, o.QEMUOpts...)...)
}

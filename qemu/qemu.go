// Copyright 2018 the u-root Authors. All rights reserved
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package qemu provides a Go API for starting QEMU VMs.
//
// qemu is mainly suitable for running QEMU-based integration tests.
//
// The environment variable `VMTEST_QEMU` overrides the path to QEMU and the
// first few arguments (defaults to "qemu"). For example:
//
//	VMTEST_QEMU='qemu-system-x86_64 -L . -m 4096 -enable-kvm'
//
// Other environment variables:
//
//	VMTEST_QEMU_ARCH (used when Options.QEMUArch is empty)
//	VMTEST_KERNEL (used when Options.Kernel is empty)
//	VMTEST_INITRAMFS (used when Options.Initramfs is empty)
package qemu

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"

	"github.com/Netflix/go-expect"
	"golang.org/x/exp/slices"
)

// ErrKernelRequiredForArgs is returned when KernelArgs is populated but Kernel is empty.
var ErrKernelRequiredForArgs = errors.New("KernelArgs can only be used when Kernel is also specified due to how QEMU bootloader works")

// ErrNoGuestArch is returned when neither Options.QEMUPath nor VMTEST_QEMU_ARCH are set.
var ErrNoGuestArch = errors.New("no QEMU guest architecture specified -- guest arch is required to decide some QEMU command-line arguments")

// ErrUnsupportedGuestArch is returned when an unsupported guest architecture value is used.
var ErrUnsupportedGuestArch = errors.New("unsupported QEMU guest architecture specified -- guest arch is required to decide some QEMU command-line arguments")

// GuestArch is the QEMU guest architecture.
type GuestArch string

const (
	GuestArchX8664   GuestArch = "x86_64"
	GuestArchI386    GuestArch = "i386"
	GuestArchAarch64 GuestArch = "aarch64"
	GuestArchArm     GuestArch = "arm"
)

// SupportedGuestArches are the supported guest architecture values.
var SupportedGuestArches = []GuestArch{
	GuestArchX8664,
	GuestArchI386,
	GuestArchAarch64,
	GuestArchArm,
}

// Valid returns whether the guest arch is a supported guest arch value.
func (g GuestArch) Valid() bool {
	return slices.Contains(SupportedGuestArches, g)
}

// Options are VM start-up parameters.
type Options struct {
	// QEMUPath is the path to the QEMU binary to invoke.
	//
	// If empty, the VMTEST_QEMU env var will be used.
	// If the env var is unspecified, "qemu" is the default.
	QEMUPath string

	// QEMUArch is the QEMU architecture used.
	//
	// Some device decisions are made based on the architecture.
	// If empty, VMTEST_QEMU_ARCH env var will be used.
	QEMUArch GuestArch

	// Path to the kernel to boot.
	//
	// If empty, VMTEST_KERNEL env var will be used.
	Kernel string

	// Path to the initramfs.
	//
	// If empty, VMTEST_INITRAMFS env var will be used.
	Initramfs string

	// Extra kernel command-line arguments.
	KernelArgs string

	// Where to send serial output.
	SerialOutput io.WriteCloser

	// Devices are devices to expose to the QEMU VM.
	Devices []Device
}

// Start starts a QEMU VM.
func (o *Options) Start() (*VM, error) {
	cmdline, err := o.Cmdline()
	if err != nil {
		return nil, err
	}

	c, err := expect.NewConsole(
		expect.WithStdout(o.SerialOutput),
		expect.WithCloser(o.SerialOutput),
	)
	if err != nil {
		return nil, err
	}

	cmd := exec.Command(cmdline[0], cmdline[1:]...)
	cmd.Stdin = c.Tty()
	cmd.Stdout = c.Tty()
	cmd.Stderr = c.Tty()
	if err := cmd.Start(); err != nil {
		return nil, err
	}

	// Close tty in parent, so that when child exits, the last reference to
	// it is gone and Console.Expect* calls automatically exit.
	c.Tty().Close()

	return &VM{
		Options: o,
		Console: c,
		cmd:     cmd,
		cmdline: cmdline,
	}, nil
}

// Arch returns the presumed guest architecture.
func (o *Options) Arch() (GuestArch, error) {
	var arch GuestArch
	if len(o.QEMUArch) > 0 {
		arch = o.QEMUArch
	} else if a := os.Getenv("VMTEST_QEMU_ARCH"); len(a) > 0 {
		arch = GuestArch(a)
	}

	if len(arch) == 0 {
		return "", ErrNoGuestArch
	}
	if !arch.Valid() {
		return "", fmt.Errorf("%w: %s", ErrUnsupportedGuestArch, arch)
	}
	return arch, nil
}

// AppendKernel appends to kernel args.
func (o *Options) AppendKernel(s string) {
	if len(o.KernelArgs) == 0 {
		o.KernelArgs = s
	} else {
		o.KernelArgs += " " + s
	}
}

// Cmdline returns the command line arguments used to start QEMU. These
// arguments are derived from the given QEMU struct.
func (o *Options) Cmdline() ([]string, error) {
	var args []string
	if len(o.QEMUPath) > 0 {
		args = append(args, o.QEMUPath)
	} else {
		// Read first few arguments for env.
		env := os.Getenv("VMTEST_QEMU")
		if env == "" {
			env = "qemu" // default
		}
		args = append(args, strings.Fields(env)...)
	}

	arch, err := o.Arch()
	if err != nil {
		return nil, err
	}

	// Disable graphics because we are using serial.
	args = append(args, "-nographic")

	// Arguments passed to the kernel:
	//
	// - earlyprintk=ttyS0: print very early debug messages to the serial
	// - console=ttyS0: /dev/console points to /dev/ttyS0 (the serial port)
	// - o.KernelArgs: extra, optional kernel arguments
	// - args required by devices
	for _, dev := range o.Devices {
		if dev != nil {
			if a := dev.KArgs(); a != nil {
				o.AppendKernel(strings.Join(a, " "))
			}
		}
	}
	var kernel string
	if len(o.Kernel) > 0 {
		kernel = o.Kernel
	} else if k := os.Getenv("VMTEST_KERNEL"); len(k) > 0 {
		kernel = k
	}

	if len(kernel) > 0 {
		args = append(args, "-kernel", kernel)
		if len(o.KernelArgs) != 0 {
			args = append(args, "-append", o.KernelArgs)
		}
	} else if len(o.KernelArgs) != 0 {
		return nil, ErrKernelRequiredForArgs
	}

	if len(o.Initramfs) != 0 {
		args = append(args, "-initrd", o.Initramfs)
	} else if i := os.Getenv("VMTEST_INITRAMFS"); len(i) > 0 {
		args = append(args, "-initrd", i)
	}

	ida := NewIDAllocator()
	for _, dev := range o.Devices {
		if dev != nil {
			if c := dev.Cmdline(arch, ida); c != nil {
				args = append(args, c...)
			}
		}
	}
	return args, nil
}

// VM is a running QEMU virtual machine.
type VM struct {
	Options *Options
	cmdline []string
	cmd     *exec.Cmd
	Console *expect.Console
}

// Cmdline is the command-line the VM was started with.
func (v *VM) Cmdline() []string {
	// Maybe return a copy?
	return v.cmdline
}

// Wait waits for the VM to exit and expects EOF from the expect console.
func (v *VM) Wait() error {
	err := v.cmd.Wait()
	if _, cerr := v.Console.ExpectEOF(); cerr != nil && err == nil {
		err = cerr
	}
	v.Console.Close()
	return err
}

// CmdlineQuoted quotes any of QEMU's command line arguments containing a space
// so it is easy to copy-n-paste into a shell for debugging.
func (v *VM) CmdlineQuoted() string {
	args := make([]string, len(v.cmdline))
	for i, arg := range v.cmdline {
		if strings.ContainsAny(arg, " \t\n") {
			args[i] = fmt.Sprintf("'%s'", arg)
		} else {
			args[i] = arg
		}
	}
	return strings.Join(args, " ")
}

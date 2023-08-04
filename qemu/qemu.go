// Copyright 2018 the u-root Authors. All rights reserved
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package qemu provides a Go API for starting QEMU VMs.
//
// qemu is mainly suitable for running QEMU-based integration tests.
//
// The environment variable `UROOT_QEMU` overrides the path to QEMU and the
// first few arguments (defaults to "qemu"). For example, I use:
//
//	UROOT_QEMU='qemu-system-x86_64 -L . -m 4096 -enable-kvm'
//
// For CI, this environment variable is set in `.circleci/images/integration/Dockerfile`.
package qemu

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/Netflix/go-expect"
)

// DefaultTimeout for `Expect` and `ExpectRE` functions.
var DefaultTimeout = 7 * time.Second

// TimeoutMultiplier increases all timeouts proportionally. Useful when running
// QEMU on a slow machine.
var TimeoutMultiplier = 1.0

func init() {
	if timeoutMultS := os.Getenv("UROOT_QEMU_TIMEOUT_X"); len(timeoutMultS) > 0 {
		t, err := strconv.ParseFloat(timeoutMultS, 64)
		if err == nil {
			TimeoutMultiplier = t
		}
	}
}

// Options are VM start-up parameters.
type Options struct {
	// QEMUPath is the path to the QEMU binary to invoke.
	//
	// If left unspecified, the VMTEST_QEMU env var will be used.
	// If the env var is unspecified, "qemu" is the default.
	QEMUPath string

	// QEMUArch is the QEMU architecture used.
	//
	// Some device decisions are made based on the architecture.
	// If left unspecified, VMTEST_QEMU_ARCH env var will be used.
	// If the env var is unspecified, the architecture default will be the
	// host arch.
	QEMUArch string

	// Path to the kernel to boot.
	Kernel string

	// Path to the initramfs.
	Initramfs string

	// Extra kernel command-line arguments.
	KernelArgs string

	// Where to send serial output.
	SerialOutput io.WriteCloser

	// Timeout is the expect timeout.
	Timeout time.Duration

	// Devices are devices to expose to the QEMU VM.
	Devices []Device
}

// Start starts a QEMU VM.
func (o *Options) Start() (*VM, error) {
	cmdline, err := o.Cmdline()
	if err != nil {
		return nil, err
	}

	c, err := expect.NewConsole(expect.WithStdout(o.SerialOutput))
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
	return &VM{
		Options: o,
		Console: c,
		cmd:     cmd,
		cmdline: cmdline,
	}, nil
}

// Arch returns the presumed guest architecture.
func (o *Options) Arch() (string, error) {
	if len(o.QEMUArch) > 0 {
		return o.QEMUArch, nil
	}
	if a := os.Getenv("VMTEST_QEMU_ARCH"); len(a) > 0 {
		return a, nil
	}
	if a, ok := GOARCHToQEMUArch[runtime.GOARCH]; ok {
		return a, nil
	}
	return "", fmt.Errorf("could not determine QEMU guest arch from VMTEST_QEMU_ARCH or GOARCH")
}

// GOARCHToQEMUArch maps GOARCH to QEMU arch values.
var GOARCHToQEMUArch = map[string]string{
	"386":   "i386",
	"amd64": "x86_64",
	"arm":   "arm",
	"arm64": "aarch64",
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
				o.KernelArgs += " " + strings.Join(a, " ")
			}
		}
	}
	if len(o.Kernel) != 0 {
		args = append(args, "-kernel", o.Kernel)
		if len(o.KernelArgs) != 0 {
			args = append(args, "-append", o.KernelArgs)
		}
	} else if len(o.KernelArgs) != 0 {
		err := fmt.Errorf("kernel args are required but cannot be added due to bootloader")
		return nil, err
	}
	if len(o.Initramfs) != 0 {
		args = append(args, "-initrd", o.Initramfs)
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

// Wait waits for the VM to exit.
func (v *VM) Wait() error {
	defer v.Console.Close()
	return v.cmd.Wait()
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

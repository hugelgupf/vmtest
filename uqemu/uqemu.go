// Copyright 2023 the u-root Authors. All rights reserved
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package uqemu provides a Go API for starting QEMU VMs with u-root initramfses.
//
// uqemu is mainly suitable for running QEMU-based integration tests.
//
// The environment variable `VMTEST_QEMU` overrides the path to QEMU and the
// first few arguments (defaults to "qemu"). For example:
//
//	VMTEST_QEMU='qemu-system-x86_64 -L . -m 4096 -enable-kvm'
//
// Other environment variables:
//
//	VMTEST_GOARCH (used when Initramfs.Env.GOARCH is empty)
//	VMTEST_KERNEL (used when Initramfs.VMOpts.Kernel is empty)
//	VMTEST_INITRAMFS_OVERRIDE (when set, use instead of building an initramfs)
package uqemu

import (
	"errors"
	"fmt"
	"os"
	"runtime"

	"github.com/hugelgupf/vmtest/qemu"
	"github.com/u-root/gobusybox/src/pkg/golang"
	"github.com/u-root/u-root/pkg/ulog"
	"github.com/u-root/u-root/pkg/uroot"
	"github.com/u-root/u-root/pkg/uroot/initramfs"
)

var ErrOutputFileSpecified = errors.New("initramfs output file must be left unspecified")

type Options struct {
	// Initramfs specifies an initramfs to be built and substituted into
	// VM.Initramfs.
	//
	// Initramfs.OutputFile must be left unspecified.
	//
	// Initramfs.Env will be filled with default values of CGO_ENABLED=0
	// and GOARCH=VMTEST_GOARCH if unspecified. It should be left
	// unspecified.
	Initramfs uroot.Opts

	// InitrdPath is the path to write the initramfs in.
	InitrdPath string

	// VM specifies the guest to start.
	//
	// VM.QEMUArch (or VMTEST_QEMU_ARCH) has to match Initramfs.Env.GOARCH
	// (or VMTEST_GOARCH) if both are specified.
	//
	// If VM.QEMUArch is unspecified, it will be derived from guest's
	// GOARCH.
	VMOpts qemu.Options
}

// GuestGOARCH returns the Guest GOARCH under test. Either VMTEST_GOARCH or
// runtime.GOARCH.
func GuestGOARCH() string {
	if env := os.Getenv("VMTEST_GOARCH"); env != "" {
		return env
	}
	return runtime.GOARCH
}

// GOARCHToQEMUArch maps GOARCH to QEMU arch values.
var GOARCHToQEMUArch = map[string]qemu.GuestArch{
	"386":   qemu.GuestArchI386,
	"amd64": qemu.GuestArchX8664,
	"arm":   qemu.GuestArchArm,
	"arm64": qemu.GuestArchAarch64,
}

// BuildInitramfs builds the specified initramfs and returns VM options with
// the created initramfs and corresponding QEMU architecture.
func (o *Options) BuildInitramfs(logger ulog.Logger) (*qemu.Options, error) {
	uopts := o.Initramfs
	if uopts.Env == nil {
		env := golang.Default()
		env.CgoEnabled = false
		env.GOARCH = GuestGOARCH()
		uopts.Env = &env
	}

	vmopts := o.VMOpts
	if override := os.Getenv("VMTEST_INITRAMFS_OVERRIDE"); len(override) > 0 {
		vmopts.Initramfs = override
	} else {
		// We're going to fill this in ourselves.
		if o.Initramfs.OutputFile != nil {
			return nil, ErrOutputFileSpecified
		}

		initrdW, err := initramfs.CPIO.OpenWriter(logger, o.InitrdPath)
		if err != nil {
			return nil, fmt.Errorf("failed to create initramfs writer: %w", err)
		}
		uopts.OutputFile = initrdW

		if err := uroot.CreateInitramfs(logger, uopts); err != nil {
			return nil, fmt.Errorf("error creating initramfs: %w", err)
		}
		vmopts.Initramfs = o.InitrdPath
	}

	if _, err := vmopts.Arch(); errors.Is(err, qemu.ErrNoGuestArch) {
		vmopts.QEMUArch = GOARCHToQEMUArch[uopts.Env.GOARCH]
	}
	return &vmopts, nil
}

// Start builds the initramfs and starts the VM guest.
func (o *Options) Start(logger ulog.Logger) (*qemu.VM, error) {
	vmopts, err := o.BuildInitramfs(logger)
	if err != nil {
		return nil, err
	}
	return vmopts.Start()
}

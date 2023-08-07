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

type options struct {
	initramfs  uroot.Opts
	initrdPath string
	logger     ulog.Logger
}

func (o *options) GOARCH() string {
	if o.initramfs.Env != nil {
		return o.initramfs.Env.GOARCH
	}
	return GuestGOARCH()
}

func (o *options) Arch() qemu.GuestArch {
	return GOARCHToQEMUArch[o.GOARCH()]
}

func (o *options) Setup(alloc *qemu.IDAllocator, opts *qemu.Options) error {
	uopts := o.initramfs
	if uopts.Env == nil {
		uopts.Env = golang.Default(golang.DisableCGO(), golang.WithGOARCH(GuestGOARCH()))
	}

	if override := os.Getenv("VMTEST_INITRAMFS_OVERRIDE"); len(override) > 0 {
		opts.Initramfs = override
	} else {
		// We're going to fill this in ourselves.
		if o.initramfs.OutputFile != nil {
			return ErrOutputFileSpecified
		}

		initrdW, err := initramfs.CPIO.OpenWriter(o.logger, o.initrdPath)
		if err != nil {
			return fmt.Errorf("failed to create initramfs writer: %w", err)
		}
		uopts.OutputFile = initrdW

		if err := uroot.CreateInitramfs(o.logger, uopts); err != nil {
			return fmt.Errorf("error creating initramfs: %w", err)
		}
		opts.Initramfs = o.initrdPath
	}

	return nil
}

// WithUrootInitramfs builds the specified initramfs and attaches it to the QEMU VM.
//
// When VMTEST_INITRAMFS_OVERRIDE is set, it foregoes building an initramfs and
// uses the initramfs path in the env variable.
//
// The arch used to build the initramfs is derived by default from
// VMTEST_GOARCH, or if unset, runtime.GOARCH (the host GOARCH).
//
// It also sets the QEMU architecture according to the Go architecture used to
// build the initramfs.
func WithUrootInitramfs(logger ulog.Logger, opts uroot.Opts, initrdPath string) qemu.ArchFn {
	return &options{
		logger:     logger,
		initramfs:  opts,
		initrdPath: initrdPath,
	}
}

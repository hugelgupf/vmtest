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
//	VMTEST_ARCH (used when Initramfs.Env.GOARCH is empty)
//	VMTEST_KERNEL (used when Initramfs.VMOpts.Kernel is empty)
//	VMTEST_INITRAMFS_OVERRIDE (when set, use instead of building an initramfs)
package uqemu

import (
	"errors"
	"fmt"
	"os"

	"github.com/hugelgupf/vmtest/qemu"
	"github.com/u-root/gobusybox/src/pkg/golang"
	"github.com/u-root/u-root/pkg/ulog"
	"github.com/u-root/u-root/pkg/uroot"
	"github.com/u-root/u-root/pkg/uroot/initramfs"
)

// ErrOutputFileSpecified is returned when uroot.Opts are supplied that already
// have an initramfs file.
var ErrOutputFileSpecified = errors.New("initramfs output file must be left unspecified")

// WithUrootInitramfs builds the specified initramfs and attaches it to the QEMU VM.
//
// When VMTEST_INITRAMFS_OVERRIDE is set, it foregoes building an initramfs and
// uses the initramfs path in the env variable.
//
// The arch used to build the initramfs is derived by default from the arch set
// in qemu.Options, which is either explicitly set, VMTEST_ARCH, or if unset,
// runtime.GOARCH (the host GOARCH).
func WithUrootInitramfs(logger ulog.Logger, uopts uroot.Opts, initrdPath string) qemu.Fn {
	return func(alloc *qemu.IDAllocator, opts *qemu.Options) error {
		if override := os.Getenv("VMTEST_INITRAMFS_OVERRIDE"); len(override) > 0 {
			opts.Initramfs = override
			return nil
		}

		if uopts.Env == nil {
			uopts.Env = golang.Default(golang.DisableCGO(), golang.WithGOARCH(string(opts.Arch())))
		}

		// We're going to fill this in ourselves.
		if uopts.OutputFile != nil {
			return ErrOutputFileSpecified
		}

		initrdW, err := initramfs.CPIO.OpenWriter(logger, initrdPath)
		if err != nil {
			return fmt.Errorf("failed to create initramfs writer: %w", err)
		}
		uopts.OutputFile = initrdW

		if err := uroot.CreateInitramfs(logger, uopts); err != nil {
			return fmt.Errorf("error creating initramfs: %w", err)
		}

		opts.Initramfs = initrdPath
		return nil
	}
}

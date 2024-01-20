// Copyright 2018 the u-root Authors. All rights reserved
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package qfirmware provides firmware configurators for use with the Go qemu
// API.
package qfirmware

import (
	"fmt"
	"os"

	"github.com/hugelgupf/vmtest/qemu"
)

// WithDefaultOVMF sets the QEMU arguments for enabling UEFI with OVMF firmware.
//
// OVMF requires the VM to be run with atleast 1 GB of memory and an machine type with smm turned on.
//
//	qemu.ArbitraryArgs("-m", "2G", "-machine", "type=q35,smm=on")
func WithDefaultOVMF() qemu.Fn {
	return WithOVMF("", "")
}

// WithOVMF sets the QEMU arguments for enabling UEFI with OVMF firmware.
//
// ovmfCode and ovmfVars are substituted by VMTEST_OVMF_CODE and VMTEST_OVMF_VARS if empty.
//
// OVMF requires the VM to be run with atleast 1 GB of memory and an machine type with msm turned on.
//
//	qemu.ArbitraryArgs("-m", "2G", "-machine", "type=q35,smm=on")
func WithOVMF(ovmfCode, ovmfVars string) qemu.Fn {
	if ovmfCode == "" {
		ovmfCode = os.Getenv("VMTEST_OVMF_CODE")
	}
	if ovmfVars == "" {
		ovmfVars = os.Getenv("VMTEST_OVMF_VARS")
	}
	return func(alloc *qemu.IDAllocator, opts *qemu.Options) error {
		opts.AppendQEMU(
			"-drive", fmt.Sprintf("if=pflash,format=raw,unit=0,file=%s,readonly=on", ovmfCode),
			"-drive", fmt.Sprintf("if=pflash,format=raw,unit=1,file=%s", ovmfVars),
		)
		return nil
	}
}

// Copyright 2022 the u-root Authors. All rights reserved
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package vmtest

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hugelgupf/vmtest/qemu"
	"github.com/hugelgupf/vmtest/qemu/qcoverage"
	"github.com/hugelgupf/vmtest/testtmp"
	"github.com/u-root/mkuimage/uimage"
)

// RunCmdsInVM starts a VM and runs the given script using gosh in the guest.
// If any command fails, the test fails.
//
// The VM can be configured with o. The kernel can be provided via o or
// VMTEST_KERNEL env var. Guest architecture can be set with VMTEST_ARCH.
//
//   - TODO: timeouts for individual individual commands.
//   - TODO: It should check their exit status. Hahaha.
func RunCmdsInVM(t testing.TB, script string, o ...Opt) {
	vm := StartVMAndRunCmds(t, script, o...)

	if _, err := vm.Console.ExpectString("TESTS PASSED MARKER"); err != nil {
		t.Errorf("Waiting for 'TESTS PASSED MARKER' failed -- script likely failed: %v", err)
	}

	if err := vm.Wait(); err != nil {
		t.Errorf("VM exited with %v", err)
	}
}

// StartVMAndRunCmds starts a VM and runs the script using gosh in the guest.
// If the commands return, the VM will be shutdown.
//
// The VM can be configured with o.
func StartVMAndRunCmds(t testing.TB, script string, o ...Opt) *qemu.VM {
	qemu.SkipWithoutQEMU(t)

	sharedDir := testtmp.TempDir(t)

	// Generate gosh shell script of test commands in o.SharedDir.
	if len(script) > 0 {
		testFile := filepath.Join(sharedDir, "test.sh")
		if err := os.WriteFile(testFile, []byte(strings.Join([]string{"set -ex", script}, "\n")), 0o777); err != nil {
			t.Fatal(err)
		}
	}

	return StartVM(t, append([]Opt{
		WithQEMUFn(
			qemu.P9Directory(sharedDir, "shelltest"),
			qcoverage.CollectKernelCoverage(t),
			qcoverage.ShareGOCOVERDIR(),
		),
		WithUimage(
			uimage.WithBusyboxCommands(
				"github.com/u-root/u-root/cmds/core/init",
				"github.com/u-root/u-root/cmds/core/gosh",
				"github.com/hugelgupf/vmtest/vminit/shutdownafter",
				"github.com/hugelgupf/vmtest/vminit/vmmount",
			),
			// Collect coverage of shelluinit.
			uimage.WithCoveredCommands(
				"github.com/hugelgupf/vmtest/vminit/shelluinit",
			),
			uimage.WithInit("init"),
			uimage.WithUinit("shutdownafter", "--", "vmmount", "--", "shelluinit"),
		),
	}, o...)...)
}

// Copyright 2022 the u-root Authors. All rights reserved
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package vmtest

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hugelgupf/vmtest/testtmp"
)

// RunCmdsInVM starts a VM and runs each command provided in testCmds in a
// shell in the VM. If any command fails, the test fails.
//
// The VM can be configured with o. The kernel can be provided via o or
// VMTEST_KERNEL env var. Guest architecture can be set with VMTEST_GOARCH.
//
// Underneath, this generates an Elvish script with these commands. The script
// is shared with the VM and run from a special init.
//
//   - TODO: timeouts for individual individual commands.
//   - TODO: It should check their exit status. Hahaha.
func RunCmdsInVM(t *testing.T, testCmds []string, o *UrootFSOptions) {
	SkipWithoutQEMU(t)

	if o == nil {
		o = &UrootFSOptions{}
	}
	if o.SharedDir == "" {
		o.SharedDir = testtmp.TempDir(t)
	}

	// Generate Elvish shell script of test commands in o.SharedDir.
	if len(testCmds) > 0 {
		testFile := filepath.Join(o.SharedDir, "test.elv")

		if err := os.WriteFile(testFile, []byte(strings.Join(testCmds, "\n")), 0o777); err != nil {
			t.Fatal(err)
		}
	}

	if len(o.BuildOpts.UinitCmd) > 0 {
		t.Fatalf("RunCmdsInVM must be able to specify a uinit; one was already specified by caller: %s", o.BuildOpts.UinitCmd)
	}
	o.BuildOpts.AddBusyBoxCommands("github.com/hugelgupf/vmtest/vminit/shelluinit")
	o.BuildOpts.UinitCmd = "shelluinit"

	vm := startVMTestVM(t, o)

	if _, err := vm.Console.ExpectString("TESTS PASSED MARKER"); err != nil {
		t.Errorf("Waiting for 'TESTS PASSED MARKER' signal: %v", err)
	}

	if err := vm.Wait(); err != nil {
		t.Errorf("VM exited with %v", err)
	}
}

package vmtest

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestCmdsInVM starts a VM, runs the commands in testCmds in a shell.
//
// TODO: It should check their exit status. Hahaha.
//
// Generates an Elvish script with these commands. The script is
// shared with the VM, and is run from the generic uinit.
func TestCmdsInVM(t *testing.T, testCmds []string, o *Options) {
	SkipWithoutQEMU(t)
	if o == nil {
		o = &Options{}
	}
	if o.TmpDir == "" {
		o.TmpDir = t.TempDir()
	}

	// Generate Elvish shell script of test commands in o.TmpDir.
	if len(testCmds) > 0 {
		testFile := filepath.Join(o.TmpDir, "test.elv")

		if err := os.WriteFile(testFile, []byte(strings.Join(testCmds, "\n")), 0o777); err != nil {
			t.Fatal(err)
		}
	}

	if len(o.Uinit) > 0 {
		t.Fatalf("TestCmdsInVM has a uinit already set")
	}
	o.Uinit = "github.com/hugelgupf/vmtest/vminit/shelluinit"

	vm, cleanup := QEMUTest(t, o)
	defer cleanup()

	if err := vm.Expect("TESTS PASSED MARKER"); err != nil {
		t.Errorf("Waiting for 'TESTS PASSED MARKER' signal: %v", err)
	}
}

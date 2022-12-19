package vmtest

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestCmdsInVM compiles the unit tests found in pkgs and runs them in a QEMU VM.
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
	o.Uinit = "github.com/hugelgupf/vmtest/testcmd/generic/uinit"

	vm, cleanup := QEMUTest(t, o)
	defer cleanup()

	if err := vm.Expect("TESTS PASSED MARKER"); err != nil {
		t.Errorf("Waiting for 'TESTS PASSED MARKER' signal: %v", err)
	}
}

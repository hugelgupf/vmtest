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
func TestCmdsInVM(t *testing.T, testCmds []string, o *UrootFSOptions) {
	SkipWithoutQEMU(t)
	if o == nil {
		o = &UrootFSOptions{}
	}
	if o.SharedDir == "" {
		o.SharedDir = t.TempDir()
	}

	// Generate Elvish shell script of test commands in o.SharedDir.
	if len(testCmds) > 0 {
		testFile := filepath.Join(o.SharedDir, "test.elv")

		if err := os.WriteFile(testFile, []byte(strings.Join(testCmds, "\n")), 0o777); err != nil {
			t.Fatal(err)
		}
	}

	if len(o.BuildOpts.UinitCmd) > 0 {
		t.Fatalf("TestCmdsInVM has a uinit already set")
	}
	o.BuildOpts.AddBusyBoxCommands("github.com/hugelgupf/vmtest/vminit/shelluinit")
	o.BuildOpts.UinitCmd = "shelluinit"

	vm := StartVMTestVM(t, o)

	if err := vm.Expect("TESTS PASSED MARKER"); err != nil {
		t.Errorf("Waiting for 'TESTS PASSED MARKER' signal: %v", err)
	}
}

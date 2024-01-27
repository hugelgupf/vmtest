package vm

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/hugelgupf/vmtest"
	"github.com/hugelgupf/vmtest/qemu"
	"github.com/u-root/u-root/pkg/uroot"
)

func TestStart(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "hello"), []byte("Hello world"), 0o777); err != nil {
		t.Fatal(err)
	}

	initramfs := uroot.Opts{
		Commands: uroot.BusyBoxCmds(
			"github.com/u-root/u-root/cmds/core/init",
			"github.com/u-root/u-root/cmds/core/ls",
			"github.com/hugelgupf/vmtest/tests/cmds/catfile",
		),
		InitCmd:   "init",
		UinitCmd:  "catfile",
		UinitArgs: []string{"-file", "/testdata/hello"},
		TempDir:   t.TempDir(),
	}

	vm := vmtest.StartVM(t,
		vmtest.WithMergedInitramfs(initramfs),
		vmtest.WithSharedDir(dir),
		vmtest.WithQEMUFn(qemu.WithVMTimeout(time.Minute)),
	)
	if _, err := vm.Console.ExpectString("Hello world"); err != nil {
		t.Error(err)
	}
	if err := vm.Wait(); err != nil {
		t.Error(err)
	}
}

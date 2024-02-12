package vm

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/hugelgupf/vmtest"
	"github.com/hugelgupf/vmtest/qemu"
	"github.com/u-root/mkuimage/uimage"
)

func TestStart(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "hello"), []byte("Hello world"), 0o777); err != nil {
		t.Fatal(err)
	}

	vm := vmtest.StartVM(t,
		vmtest.WithUimage(
			uimage.WithBusyboxCommands(
				"github.com/u-root/u-root/cmds/core/init",
				"github.com/u-root/u-root/cmds/core/ls",
				"github.com/hugelgupf/vmtest/tests/cmds/catfile",
			),
			uimage.WithInit("init"),
			uimage.WithUinit("catfile", "-file", "/testdata/hello"),
		),
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

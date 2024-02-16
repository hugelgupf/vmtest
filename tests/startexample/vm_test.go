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
				"github.com/u-root/u-root/cmds/core/cat",
				"github.com/hugelgupf/vmtest/vminit/shutdownafter",
				"github.com/hugelgupf/vmtest/vminit/vmmount",
			),
			uimage.WithInit("init"),
			uimage.WithUinit("shutdownafter", "--", "vmmount", "--", "cat", "/mount/9p/testdir/hello"),
		),
		vmtest.WithQEMUFn(
			qemu.WithVMTimeout(time.Minute),
			qemu.P9Directory(dir, "testdir"),
		),
	)
	if _, err := vm.Console.ExpectString("Hello world"); err != nil {
		t.Error(err)
	}
	if err := vm.Wait(); err != nil {
		t.Error(err)
	}
}

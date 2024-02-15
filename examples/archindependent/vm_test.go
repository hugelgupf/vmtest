package vm_x_test

import (
	"testing"

	"github.com/hugelgupf/vmtest/qemu"
	"github.com/hugelgupf/vmtest/uqemu"
	"github.com/u-root/mkuimage/uimage"
)

func TestVM(t *testing.T) {
	// Runs echo "Hello world" and then kernel panics as init quits.
	vm := qemu.StartT(t,
		"vm",
		qemu.ArchUseEnvv,
		uqemu.WithUimageT(t,
			uimage.WithBusyboxCommands(
				"github.com/u-root/u-root/cmds/core/init",
				"github.com/u-root/u-root/cmds/core/echo",
			),
			uimage.WithInit("init"),
			uimage.WithUinit("echo", "Hello world"),
		),
		qemu.HaltOnKernelPanic(),
	)

	if _, err := vm.Console.ExpectString("Hello world"); err != nil {
		t.Fatalf("Failed to get Hello world: %v", err)
	}

	if err := vm.Wait(); err != nil {
		t.Fatalf("Wait: %v", err)
	}
}

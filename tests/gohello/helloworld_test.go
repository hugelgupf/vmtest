package helloworld

import (
	"testing"

	"github.com/hugelgupf/vmtest"
	"github.com/hugelgupf/vmtest/guest"
	"github.com/hugelgupf/vmtest/qemu"
)

func TestStartVM(t *testing.T) {
	vmtest.SkipWithoutQEMU(t)

	vmtest.RunGoTestsInVM(t, []string{"github.com/hugelgupf/vmtest/tests/gohello"}, vmtest.WithVMOpt(
		vmtest.WithBusyboxCommands(
			"github.com/u-root/u-root/cmds/core/dhclient",
			"github.com/u-root/u-root/cmds/core/ls",
			"github.com/u-root/u-root/cmds/core/false",
		),
		vmtest.WithQEMUFn(
			qemu.VirtioRandom(),
		),
	))
}

func TestHelloWorld(t *testing.T) {
	guest.SkipIfNotInVM(t)

	t.Logf("Hello world")
}

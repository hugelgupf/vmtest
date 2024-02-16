package helloworld

import (
	"testing"

	"github.com/hugelgupf/vmtest/govmtest"
	"github.com/hugelgupf/vmtest/guest"
	"github.com/hugelgupf/vmtest/qemu"
	"github.com/u-root/mkuimage/uimage"
)

func TestStartVM(t *testing.T) {
	qemu.SkipWithoutQEMU(t)

	govmtest.Run(t, "vm",
		govmtest.WithPackageToTest("github.com/hugelgupf/vmtest/tests/gohello"),
		govmtest.WithUimage(
			uimage.WithBusyboxCommands(
				"github.com/u-root/u-root/cmds/core/dhclient",
				"github.com/u-root/u-root/cmds/core/ls",
				"github.com/u-root/u-root/cmds/core/false",
			),
		),
		govmtest.WithQEMUFn(
			qemu.VirtioRandom(),
		),
	)
}

func TestHelloWorld(t *testing.T) {
	guest.SkipIfNotInVM(t)

	t.Logf("Hello world")
}

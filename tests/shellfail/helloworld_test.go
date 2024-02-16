package helloworld

import (
	"testing"

	"github.com/hugelgupf/vmtest/internal/failtesting"
	"github.com/hugelgupf/vmtest/qemu"
	"github.com/hugelgupf/vmtest/scriptvm"
	"github.com/u-root/mkuimage/uimage"
)

func TestStartVM(t *testing.T) {
	qemu.SkipWithoutQEMU(t)

	ft := &failtesting.TB{TB: t}
	scriptvm.Run(ft, "vm", "false", scriptvm.WithUimage(
		uimage.WithBusyboxCommands("github.com/u-root/u-root/cmds/core/false"),
	))

	if !ft.HasFailed {
		t.Error("Shell VM test did not fail as expected.")
	}
}

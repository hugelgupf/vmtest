package helloworld

import (
	"testing"

	"github.com/hugelgupf/vmtest"
	"github.com/hugelgupf/vmtest/internal/failtesting"
)

func TestStartVM(t *testing.T) {
	vmtest.SkipWithoutQEMU(t)

	ft := &failtesting.TB{TB: t}
	vmtest.RunCmdsInVM(ft, "false", vmtest.WithBusyboxCommands("github.com/u-root/u-root/cmds/core/false"))

	if !ft.HasFailed {
		t.Error("Shell VM test did not fail as expected.")
	}
}

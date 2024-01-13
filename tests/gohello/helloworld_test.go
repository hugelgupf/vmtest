package helloworld

import (
	"testing"

	"github.com/hugelgupf/vmtest"
	"github.com/hugelgupf/vmtest/guest"
)

func TestStartVM(t *testing.T) {
	vmtest.SkipWithoutQEMU(t)

	vmtest.RunGoTestsInVM(t, []string{"github.com/hugelgupf/vmtest/tests/gohello"}, vmtest.WithVMOpt(vmtest.WithBusyboxCommands(
		"github.com/u-root/u-root/cmds/core/dhclient",
		"github.com/u-root/u-root/cmds/core/elvish",
		"github.com/u-root/u-root/cmds/core/false",
	)))
}

func TestHelloWorld(t *testing.T) {
	guest.SkipIfNotInVM(t)

	t.Logf("Hello world")
}

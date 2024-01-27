package helloworld

import (
	"os"
	"regexp"
	"testing"

	"github.com/hugelgupf/vmtest"
	"github.com/hugelgupf/vmtest/qemu"
	"github.com/hugelgupf/vmtest/testtmp"
	"github.com/u-root/gobusybox/src/pkg/golang"
)

func TestStartVM(t *testing.T) {
	vmtest.SkipWithoutQEMU(t)

	goCov := os.Getenv("GOCOVERDIR")
	if goCov == "" {
		goCov = testtmp.TempDir(t)
		t.Setenv("GOCOVERDIR", goCov)
	}

	// Kernel coverage is copied to kcovDir during t.Cleanup, so induce it
	// before the test is over by using a sub-test.
	t.Run("test", func(t *testing.T) {
		vmtest.RunCmdsInVM(t, "donothing",
			vmtest.WithGoBuildOpts(&golang.BuildOpts{
				ExtraArgs: []string{"-cover", "-coverpkg=all", "-covermode=atomic"},
			}),
			vmtest.WithBusyboxCommands(
				"github.com/hugelgupf/vmtest/tests/cmds/donothing",
			),
		)
	})

	env := golang.Default(golang.DisableCGO(), golang.WithGOARCH(string(qemu.GuestArch())))
	cmd := env.GoCmd("tool", "covdata", "func", "-i="+goCov)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Errorf("go tool covdata: %v", err)
	}

	// GOCOVERDIR should have `show` coverage.
	matched, err := regexp.Match(`github.com/hugelgupf/vmtest/tests/cmds/donothing/main.go:\d+:\s+show\s+100.0%`, out)
	if err != nil {
		t.Error(err)
	} else if !matched {
		t.Errorf("GOCOVERDIR should contain 100%% coverage of donothing's show")
	}
}

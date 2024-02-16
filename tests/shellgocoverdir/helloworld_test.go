package helloworld

import (
	"os"
	"regexp"
	"testing"

	"github.com/hugelgupf/vmtest"
	"github.com/hugelgupf/vmtest/qemu"
	"github.com/hugelgupf/vmtest/testtmp"
	"github.com/u-root/gobusybox/src/pkg/golang"
	"github.com/u-root/mkuimage/uimage"
)

func TestStartVM(t *testing.T) {
	qemu.SkipWithoutQEMU(t)

	for _, script := range []string{
		"donothing",
		// Test that even an when GOCOVERDIR is not unmounted properly,
		// the data is there.
		"donothing\nsync\necho \"TESTS PASSED MARKER\"\nshutdown",
	} {
		t.Run(script, func(t *testing.T) {
			goCov := os.Getenv("VMTEST_GOCOVERDIR")
			if goCov == "" {
				goCov = testtmp.TempDir(t)
				t.Setenv("VMTEST_GOCOVERDIR", goCov)
			}

			vmtest.RunCmdsInVM(t, script,
				vmtest.WithUimage(
					uimage.WithCoveredCommands(
						"github.com/hugelgupf/vmtest/tests/cmds/donothing",
					),
					uimage.WithBusyboxCommands(
						"github.com/u-root/u-root/cmds/core/sync",
						"github.com/u-root/u-root/cmds/core/shutdown",
					),
				),
			)

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
		})
	}
}

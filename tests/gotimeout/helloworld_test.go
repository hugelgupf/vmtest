package helloworld

import (
	"os"
	"testing"
	"time"

	"github.com/hugelgupf/vmtest/govmtest"
	"github.com/hugelgupf/vmtest/internal/cover"
	"github.com/hugelgupf/vmtest/internal/failtesting"
	"github.com/hugelgupf/vmtest/qemu"
)

func TestStartVM(t *testing.T) {
	qemu.SkipWithoutQEMU(t)

	ft := &failtesting.TB{TB: t}
	govmtest.Run(ft, "vm",
		govmtest.WithPackageToTest("github.com/hugelgupf/vmtest/tests/gotimeout"),
		govmtest.WithGoTestTimeout(2*time.Second),
		govmtest.WithUimage(cover.WithCoverInstead("github.com/hugelgupf/vmtest/vminit/gouinit")),
	)

	if !ft.HasFailed {
		t.Error("Go VM test did not fail as expected.")
	}
}

func TestMain(m *testing.M) {
	if os.Getenv("VMTEST_IN_GUEST") == "1" {
		time.Sleep(10 * time.Second)
	}
	os.Exit(m.Run())
}

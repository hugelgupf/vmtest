package fail

import (
	"os"
	"testing"

	"github.com/hugelgupf/vmtest/govmtest"
	"github.com/hugelgupf/vmtest/internal/failtesting"
	"github.com/hugelgupf/vmtest/qemu"
)

func TestStartVM(t *testing.T) {
	qemu.SkipWithoutQEMU(t)

	ft := &failtesting.TB{TB: t}
	govmtest.Run(ft, "vm", govmtest.WithPackageToTest("github.com/hugelgupf/vmtest/tests/gofail"))

	if !ft.HasFailed {
		t.Error("Go VM test did not fail as expected.")
	}
}

func TestMain(m *testing.M) {
	if os.Getenv("VMTEST_IN_GUEST") == "1" {
		os.Exit(6)
	}
	os.Exit(m.Run())
}

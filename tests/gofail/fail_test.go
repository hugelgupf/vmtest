package fail

import (
	"os"
	"testing"

	"github.com/hugelgupf/vmtest"
	"github.com/hugelgupf/vmtest/internal/failtesting"
)

func TestStartVM(t *testing.T) {
	vmtest.SkipWithoutQEMU(t)

	ft := &failtesting.TB{TB: t}
	vmtest.RunGoTestsInVM(ft, []string{"github.com/hugelgupf/vmtest/tests/gofail"})

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

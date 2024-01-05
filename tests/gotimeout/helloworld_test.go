package helloworld

import (
	"os"
	"testing"
	"time"

	"github.com/hugelgupf/vmtest"
	"github.com/hugelgupf/vmtest/internal/failtesting"
)

func TestStartVM(t *testing.T) {
	vmtest.SkipWithoutQEMU(t)

	ft := &failtesting.TB{TB: t}
	vmtest.RunVM(ft, vmtest.RunGoTests(vmtest.GoTestPackage("github.com/hugelgupf/vmtest/tests/gofail"), vmtest.WithGoTestTimeout(2*time.Second)))

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

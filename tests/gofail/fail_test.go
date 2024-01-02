package fail

import (
	"fmt"
	"os"
	"testing"

	"github.com/hugelgupf/vmtest"
)

type failTesting struct {
	testing.TB

	errorf string
	failed bool
}

func (f *failTesting) Errorf(format string, args ...any) {
	f.errorf = fmt.Sprintf(format, args...)
	f.failed = true
	f.TB.Logf("ERRORF: "+format, args...)
}

func (f *failTesting) Fatalf(format string, args ...any) {
	f.errorf = fmt.Sprintf(format, args...)
	f.failed = true
	f.TB.Skipf("FATALF: "+format, args...)
}

func TestStartVM(t *testing.T) {
	vmtest.SkipWithoutQEMU(t)

	ft := &failTesting{TB: t}
	vmtest.RunGoTestsInVM(ft, []string{"github.com/hugelgupf/vmtest/tests/gofail"})

	if !ft.failed {
		t.Error("Go VM test did not fail as expected.")
	}
}

func TestMain(m *testing.M) {
	if os.Getenv("VMTEST_IN_GUEST") == "1" {
		os.Exit(6)
	}
	os.Exit(m.Run())
}

package helloworld

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/hugelgupf/vmtest"
	"github.com/hugelgupf/vmtest/guest"
	"github.com/hugelgupf/vmtest/testtmp"
)

func TestStartVM(t *testing.T) {
	vmtest.SkipWithoutQEMU(t)

	kcovDir := os.Getenv("VMTEST_KERNEL_COVERAGE_DIR")
	if kcovDir == "" {
		kcovDir = testtmp.TempDir(t)
		os.Setenv("VMTEST_KERNEL_COVERAGE_DIR", kcovDir)
	}

	// Kernel coverage is copied to kcovDir during t.Cleanup, so induce it
	// before the test is over by using a sub-test.
	t.Run("test", func(t *testing.T) {
		vmtest.RunGoTestsInVM(t, []string{"github.com/hugelgupf/vmtest/tests/gohello"})
	})

	if _, err := os.Stat(filepath.Join(kcovDir, "TestStartVM", "test", "0", "kernel_coverage.tar")); err != nil {
		t.Fatalf("Kernel coverage file not found: %v", err)
	}
}

func TestHelloWorld(t *testing.T) {
	guest.SkipIfNotInVM(t)

	t.Logf("Hello world")
}

package helloworld

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/hugelgupf/vmtest"
	"github.com/hugelgupf/vmtest/qemu"
	"github.com/hugelgupf/vmtest/testtmp"
)

func TestStartVM(t *testing.T) {
	// riscv64 kernel coverage not working
	qemu.SkipIfNotArch(t, qemu.ArchAMD64, qemu.ArchArm, qemu.ArchArm64)
	qemu.SkipWithoutQEMU(t)

	kcovDir := os.Getenv("VMTEST_KERNEL_COVERAGE_DIR")
	if kcovDir == "" {
		kcovDir = testtmp.TempDir(t)
		os.Setenv("VMTEST_KERNEL_COVERAGE_DIR", kcovDir)
	}

	// Kernel coverage is copied to kcovDir during t.Cleanup, so induce it
	// before the test is over by using a sub-test.
	t.Run("test", func(t *testing.T) {
		vmtest.RunCmdsInVM(t, `echo "Hello World"`)
	})

	if _, err := os.Stat(filepath.Join(kcovDir, "TestStartVM", "test", "0", "kernel_coverage.tar")); err != nil {
		t.Fatalf("Kernel coverage file not found: %v", err)
	}
}

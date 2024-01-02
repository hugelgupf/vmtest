package vm_x_test

import (
	"testing"

	"github.com/hugelgupf/vmtest"
	"github.com/hugelgupf/vmtest/internal/failtesting"
	"github.com/hugelgupf/vmtest/qemu"
)

// Clears all previously added args.
func clearArgs() qemu.Fn {
	return func(alloc *qemu.IDAllocator, opts *qemu.Options) error {
		opts.QEMUArgs = nil
		opts.KernelArgs = ""
		// In case the user is calling this test with env vars set.
		opts.Kernel = ""
		opts.Initramfs = ""
		return nil
	}
}

func TestStartNotWait(t *testing.T) {
	var ft *failtesting.TB
	var vm *qemu.VM
	t.Run("test", func(t *testing.T) {
		ft = &failtesting.TB{TB: t}
		vm = vmtest.StartVM(ft, vmtest.WithQEMUFn(qemu.WithQEMUCommand("sleep 2"), clearArgs()))
	})

	if !ft.HasFailed {
		t.Errorf("Test should have failed for not waiting on VM")
	}

	if err := vm.Wait(); err != nil {
		t.Fatal(err)
	}
}

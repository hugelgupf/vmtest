package helloworld

import (
	"testing"

	"github.com/hugelgupf/vmtest"
	"github.com/hugelgupf/vmtest/qemu"
)

func TestStartVM(t *testing.T) {
	qemu.SkipWithoutQEMU(t)

	vmtest.RunCmdsInVM(t, `echo "Hello World"`)
}

package helloworld

import (
	"testing"

	"github.com/hugelgupf/vmtest/qemu"
	"github.com/hugelgupf/vmtest/scriptvm"
)

func TestStartVM(t *testing.T) {
	qemu.SkipWithoutQEMU(t)

	scriptvm.Run(t, "vm", `echo "Hello World"`)
}

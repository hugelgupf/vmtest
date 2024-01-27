package helloworld

import (
	"testing"

	"github.com/hugelgupf/vmtest"
)

func TestStartVM(t *testing.T) {
	vmtest.SkipWithoutQEMU(t)

	vmtest.RunCmdsInVM(t, `echo "Hello World"`)
}

package helloworld

import (
	"testing"

	"github.com/hugelgupf/vmtest"
)

func TestStartVM(t *testing.T) {
	vmtest.RunCmdsInVM(t, []string{`echo "Hello World"`})
}

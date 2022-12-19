package helloworld

import (
	"testing"

	"github.com/hugelgupf/vmtest"
	"github.com/u-root/u-root/pkg/testutil"
)

func TestStartVM(t *testing.T) {
	vmtest.GolangTest(t, []string{"github.com/hugelgupf/vmtest/tests/gohello"}, nil)
}

func TestHelloWorld(t *testing.T) {
	testutil.SkipIfNotRoot(t)

	t.Logf("Hello world")
}

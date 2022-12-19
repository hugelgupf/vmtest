package helloworld

import (
	"testing"

	"github.com/hugelgupf/vmtest"
	"github.com/u-root/u-root/pkg/testutil"
)

func TestStartVM(t *testing.T) {
	// Run the read-write tests from fsimpl/test/rwvm.
	vmtest.GolangTest(t, []string{"github.com/hugelgupf/vmtest/tests/helloworld"}, nil)
}

func TestHelloWorld(t *testing.T) {
	testutil.SkipIfNotRoot(t)

	t.Logf("Hello world")
}

package helloworld

import (
	"testing"

	"github.com/hugelgupf/vmtest"
)

func TestStartVM(t *testing.T) {
	vmtest.RunGoTestsInVM(t, []string{"github.com/hugelgupf/vmtest/tests/gohello"}, nil)
}

func TestHelloWorld(t *testing.T) {
	vmtest.SkipIfNotInVM(t)

	t.Logf("Hello world")
}

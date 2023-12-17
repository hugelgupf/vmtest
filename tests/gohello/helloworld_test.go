package helloworld

import (
	"testing"

	"github.com/hugelgupf/vmtest"
	"github.com/hugelgupf/vmtest/guest"
)

func TestStartVM(t *testing.T) {
	vmtest.RunGoTestsInVM(t, []string{"github.com/hugelgupf/vmtest/tests/gohello"})
}

func TestHelloWorld(t *testing.T) {
	guest.SkipIfNotInVM(t)

	t.Logf("Hello world")
}

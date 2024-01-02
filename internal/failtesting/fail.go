// Package failtesting can be used to expect test failure in a Go test.
package failtesting

import (
	"fmt"
	"testing"
)

// TB records test failures.
type TB struct {
	testing.TB

	ErrorValue string
	HasFailed  bool
}

// Errorf implements testing.TB.Errorf by logging an error, but not failing the
// underlying test.
func (t *TB) Errorf(format string, args ...any) {
	t.ErrorValue = fmt.Sprintf(format, args...)
	t.HasFailed = true
	t.TB.Logf("ERRORF: "+format, args...)
}

// Fatalf implements testing.TB.Fatalf by logging an error and skipping the
// remainder of the test, but not failing the underlying test.
func (t *TB) Fatalf(format string, args ...any) {
	t.ErrorValue = fmt.Sprintf(format, args...)
	t.HasFailed = true
	t.TB.Skipf("FATALF: "+format, args...)
}

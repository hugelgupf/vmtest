package bench

import (
	"testing"

	"github.com/hugelgupf/vmtest"
)

func TestRunBenchmarkInVM(t *testing.T) {
	vmtest.RunGoTestsInVM(t, []string{"github.com/hugelgupf/vmtest/tests/gobench"}, nil)
}

func fib(n int) int {
	if n < 2 {
		return n
	}
	return fib(n-1) + fib(n-2)
}

func BenchmarkFib10(b *testing.B) {
	vmtest.SkipIfNotInVM(b)

	for n := 0; n < b.N; n++ {
		fib(10)
	}
}

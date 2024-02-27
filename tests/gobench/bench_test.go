package bench

import (
	"testing"

	"github.com/hugelgupf/vmtest/govmtest"
	"github.com/hugelgupf/vmtest/guest"
	"github.com/hugelgupf/vmtest/internal/cover"
)

func TestRunBenchmarkInVM(t *testing.T) {
	govmtest.Run(t, "vm",
		govmtest.WithPackageToTest("github.com/hugelgupf/vmtest/tests/gobench"),
		govmtest.WithUimage(cover.WithCoverInstead("github.com/hugelgupf/vmtest/vminit/gouinit")),
	)
}

func fib(n int) int {
	if n < 2 {
		return n
	}
	return fib(n-1) + fib(n-2)
}

func BenchmarkFib10(b *testing.B) {
	guest.SkipIfNotInVM(b)

	for n := 0; n < b.N; n++ {
		fib(10)
	}
}

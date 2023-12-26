package helloworld

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/hugelgupf/vmtest"
	"github.com/hugelgupf/vmtest/guest"
	"github.com/hugelgupf/vmtest/testtmp"
)

func TestStartVM(t *testing.T) {
	vmtest.SkipWithoutQEMU(t)

	goProfile := os.Getenv("VMTEST_GO_PROFILE")
	if goProfile == "" {
		goProfile = filepath.Join(testtmp.TempDir(t), "coverage.txt")
		t.Setenv("VMTEST_GO_PROFILE", goProfile)
	}

	goCov := os.Getenv("GOCOVERDIR")
	if goCov == "" {
		goCov = testtmp.TempDir(t)
		t.Setenv("GOCOVERDIR", goCov)
	}

	t.Run("test", func(t *testing.T) {
		vmtest.RunGoTestsInVM(t, []string{"github.com/hugelgupf/vmtest/tests/gocover"})
	})

	if fi, err := os.Stat(goProfile); err != nil {
		t.Fatalf("Go coverage file not found: %v", err)
	} else if fi.Size() == 0 {
		t.Fatalf("No coverage found")
	}

	dirs, err := os.ReadDir(goCov)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range dirs {
		t.Logf("file in GOCOVERDIR: %s", e.Name())
	}
	if len(dirs) < 2 {
		t.Errorf("Go coverage dir should have at least 2 files generated")
	}
}

func TestHelloWorld(t *testing.T) {
	guest.SkipIfNotInVM(t)

	// In case TestMain/run get messed up and there's an infinite loop.
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	c := exec.CommandContext(ctx, "/proc/self/exe")
	c.Env = append(os.Environ(), "VMTEST_GOCOVERTEST_HELLO=1")

	var s strings.Builder
	c.Stdout, c.Stderr = &s, &s
	if err := c.Run(); err != nil {
		t.Fatalf("Could not run self: %v", err)
	}

	got := s.String()
	t.Logf("Output: %s", got)
	if want := "Hello world!\n"; got != want {
		t.Errorf("Got %s, want %s", got, want)
	}
}

func run(m *testing.M) int {
	if os.Getenv("VMTEST_GOCOVERTEST_HELLO") != "" {
		// Some code to cover.
		Hello()
		return 0
	}
	return m.Run()
}

func TestMain(m *testing.M) {
	os.Exit(run(m))
}

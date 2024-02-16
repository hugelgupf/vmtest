package helloworld

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/hugelgupf/vmtest/govmtest"
	"github.com/hugelgupf/vmtest/guest"
	"github.com/hugelgupf/vmtest/qemu"
	"github.com/hugelgupf/vmtest/testtmp"
	"github.com/u-root/gobusybox/src/pkg/golang"
)

func TestStartVM(t *testing.T) {
	qemu.SkipWithoutQEMU(t)

	goProfile := os.Getenv("VMTEST_GO_PROFILE")
	if goProfile == "" {
		goProfile = filepath.Join(testtmp.TempDir(t), "coverage.txt")
		t.Setenv("VMTEST_GO_PROFILE", goProfile)
	}

	goCov := os.Getenv("VMTEST_GOCOVERDIR")
	if goCov == "" {
		goCov = testtmp.TempDir(t)
		t.Setenv("VMTEST_GOCOVERDIR", goCov)
	}

	t.Run("test", func(t *testing.T) {
		govmtest.Run(t, "vm", govmtest.WithPackageToTest("github.com/hugelgupf/vmtest/tests/gocover"))
	})

	// Check VMTEST_GO_PROFILE coverage collected.
	if fi, err := os.Stat(goProfile); err != nil {
		t.Fatalf("Go coverage file not found: %v", err)
	} else if fi.Size() == 0 {
		t.Fatalf("No coverage found")
	}

	env := golang.Default(golang.DisableCGO(), golang.WithGOARCH(string(qemu.GuestArch())))
	cmd := env.GoCmd("tool", "cover", "-func="+goProfile)
	out, err := cmd.CombinedOutput()
	t.Logf("go tool cover -func=%s:\n%s", goProfile, string(out))
	if err != nil {
		t.Errorf("go tool cover: %v", err)
	}

	// World should be covered by this.
	matched, err := regexp.Match(`github.com/hugelgupf/vmtest/tests/gocover/helloworld.go:\d+:\s+World\s+100.0%`, out)
	if err != nil {
		t.Error(err)
	} else if !matched {
		t.Errorf("Coverage file should contain 100%% coverage of World")
	}

	// Check GOCOVERDIR coverage collected.
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

	cmd = env.GoCmd("tool", "covdata", "func", "-i="+goCov)
	out, err = cmd.CombinedOutput()
	t.Logf("go tool covdata func %s:\n%s", goCov, string(out))
	if err != nil {
		t.Errorf("go tool covdata: %v", err)
	}

	// GOCOVERDIR should have Hello coverage.
	matched, err = regexp.Match(`github.com/hugelgupf/vmtest/tests/gocover/helloworld.go:\d+:\s+Hello\s+100.0%`, out)
	if err != nil {
		t.Error(err)
	} else if !matched {
		t.Errorf("GOCOVERDIR should contain 100%% coverage of Hello")
	}
}

// Coverage of Hello() should appear in the data collected by GOCOVERDIR.
func TestHello(t *testing.T) {
	guest.SkipIfNotInVM(t)

	// In case TestMain/run get messed up and there's an infinite loop.
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
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

// Coverage of World should appear in the coverprofile text file.
func TestWorld(t *testing.T) {
	guest.SkipIfNotInVM(t)

	World()
}

func run(m *testing.M) int {
	if os.Getenv("VMTEST_GOCOVERTEST_HELLO") == "1" {
		// Some code to cover.
		Hello()
		return 0
	}
	return m.Run()
}

func TestMain(m *testing.M) {
	os.Exit(run(m))
}

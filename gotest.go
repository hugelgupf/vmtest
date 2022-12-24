// Copyright 2022 the u-root Authors. All rights reserved
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package vmtest

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"testing"

	"github.com/hugelgupf/vmtest/internal/json2test"
	"github.com/u-root/u-root/pkg/golang"
	"github.com/u-root/u-root/pkg/uio"
	"github.com/u-root/u-root/pkg/uroot"
)

// GolangTest compiles the tests found in pkgs and runs them in a QEMU VM
// configured in options `o`. It collects the test results and provides a
// pass/fail result of each individual test.
//
// GolangTest runs tests and benchmarks, but not fuzz tests. (TODO:
// Configuration for this.)
//
// TODO: test timeout.
// TODO: check each test's exit status.
func GolangTest(t *testing.T, pkgs []string, o *UrootFSOptions) {
	SkipWithoutQEMU(t)

	// TODO: support arm
	if GoTestArch() != "amd64" && GoTestArch() != "arm64" {
		t.Skipf("test not supported on %s", GoTestArch())
	}

	if o == nil {
		o = &UrootFSOptions{}
	}
	if o.SharedDir == "" {
		o.SharedDir = t.TempDir()
	}

	vmCoverProfile, ok := os.LookupEnv("UROOT_QEMU_COVERPROFILE")
	if !ok {
		t.Log("QEMU test coverage is not collected unless UROOT_QEMU_COVERPROFILE is set")
	}

	// Set up u-root build options.
	env := golang.Default()
	env.CgoEnabled = false
	env.GOARCH = GoTestArch()
	o.BuildOpts.Env = env

	// Statically build tests and add them to the temporary directory.
	var tests []string
	testDir := filepath.Join(o.SharedDir, "tests")

	if len(vmCoverProfile) > 0 {
		f, err := os.Create(filepath.Join(o.SharedDir, "coverage.profile"))
		if err != nil {
			t.Fatalf("Could not create coverage file %v", err)
		}
		if err := f.Close(); err != nil {
			t.Fatalf("Could not close coverage.profile: %v", err)
		}
	}

	// Compile the Go tests. Place the test binaries in a directory that
	// will be shared with the VM using 9P.
	for _, pkg := range pkgs {
		pkgDir := filepath.Join(testDir, pkg)
		if err := os.MkdirAll(pkgDir, 0o755); err != nil {
			t.Fatal(err)
		}

		testFile := filepath.Join(pkgDir, fmt.Sprintf("%s.test", path.Base(pkg)))

		args := []string{
			"test",
			"-gcflags=all=-l",
			"-ldflags", "-s -w",
			"-c", pkg,
			"-o", testFile,
		}
		if len(vmCoverProfile) > 0 {
			args = append(args, "-covermode=atomic")
		}

		// TODO: replace this with usage of golang package.
		cmd := exec.Command("go", args...)
		cmd.Env = append(os.Environ(), "CGO_ENABLED=0")
		if stderr, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("could not build %s: %v\n%s", pkg, err, string(stderr))
		}

		// When a package does not contain any tests, the test
		// executable is not generated, so it is not included in the
		// `tests` list.
		if _, err := os.Stat(testFile); !os.IsNotExist(err) {
			tests = append(tests, pkg)

			p, err := o.BuildOpts.Env.Package(pkg)
			if err != nil {
				t.Fatal(err)
			}
			// Optimistically copy any files in the pkg's
			// directory, in case e.g. a testdata dir is there.
			if err := copyRelativeFiles(p.Dir, filepath.Join(testDir, pkg)); err != nil {
				t.Fatal(err)
			}
		}
	}

	// Add some necessary commands to the VM.
	o.BuildOpts.AddBusyBoxCommands("github.com/u-root/u-root/cmds/core/dhclient", "github.com/hugelgupf/vmtest/vminit/gouinit")
	o.BuildOpts.AddCommands(uroot.BinaryCmds("cmd/test2json")...)

	// Specify the custom gotest uinit, which will mount the 9P file system
	// and run the tests from there.
	o.BuildOpts.UinitCmd = "gouinit"

	tc := json2test.NewTestCollector()
	serial := []io.Writer{
		// Collect JSON test events in tc.
		json2test.EventParser(tc),
		// Write non-JSON output to log.
		JSONLessTestLineWriter(t, "serial"),
	}
	if o.QEMUOpts.SerialOutput != nil {
		serial = append(serial, o.QEMUOpts.SerialOutput)
	}
	o.QEMUOpts.SerialOutput = uio.MultiWriteCloser(serial...)
	if len(vmCoverProfile) > 0 {
		o.QEMUOpts.KernelArgs += " uroot.uinitargs=-coverprofile=/testdata/coverage.profile"
	}

	// Create the initramfs and start the VM.
	vm := StartVMTestVM(t, o)

	if err := vm.Expect("TESTS PASSED MARKER"); err != nil {
		t.Errorf("Waiting for 'TESTS PASSED MARKER' signal: %v", err)
	}

	// Collect Go coverage.
	if len(vmCoverProfile) > 0 {
		cov, err := os.Open(filepath.Join(o.SharedDir, "coverage.profile"))
		if err != nil {
			t.Fatalf("No coverage file shared from VM: %v", err)
		}

		out, err := os.OpenFile(vmCoverProfile, os.O_CREATE|os.O_RDWR|os.O_APPEND, 0644)
		if err != nil {
			t.Fatalf("Could not open vmcoverageprofile: %v", err)
		}

		if _, err := io.Copy(out, cov); err != nil {
			t.Fatalf("Error copying coverage: %s", err)
		}
		if err := out.Close(); err != nil {
			t.Fatalf("Could not close vmcoverageprofile: %v", err)
		}
		if err := cov.Close(); err != nil {
			t.Fatalf("Could not close coverage.profile: %v", err)
		}
	}

	// TODO: check that tc.Tests == tests
	for pkg, test := range tc.Tests {
		switch test.State {
		case json2test.StateFail:
			t.Errorf("Test %v failed:\n%v", pkg, test.FullOutput)
		case json2test.StateSkip:
			t.Logf("Test %v skipped", pkg)
		case json2test.StatePass:
			// Nothing.
		default:
			t.Errorf("Test %v left in state %v:\n%v", pkg, test.State, test.FullOutput)
		}
	}
}

func copyRelativeFiles(src string, dst string) error {
	return filepath.Walk(src, func(path string, fi os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}

		if fi.Mode().IsDir() {
			return os.MkdirAll(filepath.Join(dst, rel), fi.Mode().Perm())
		} else if fi.Mode().IsRegular() {
			srcf, err := os.Open(path)
			if err != nil {
				return err
			}
			defer srcf.Close()
			dstf, err := os.Create(filepath.Join(dst, rel))
			if err != nil {
				return err
			}
			defer dstf.Close()
			_, err = io.Copy(dstf, srcf)
			return err
		}
		return nil
	})
}

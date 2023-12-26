// Copyright 2022 the u-root Authors. All rights reserved
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package vmtest

import (
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"testing"

	"github.com/hugelgupf/vmtest/internal/json2test"
	"github.com/hugelgupf/vmtest/qemu"
	"github.com/hugelgupf/vmtest/testtmp"
	"github.com/u-root/gobusybox/src/pkg/golang"
	"github.com/u-root/u-root/pkg/uroot"
	"golang.org/x/tools/go/packages"
)

func lookupPkgs(env golang.Environ, dir string, patterns ...string) ([]*packages.Package, error) {
	cfg := &packages.Config{
		Mode:  packages.NeedName | packages.NeedFiles,
		Env:   append(os.Environ(), env.Env()...),
		Dir:   dir,
		Tests: true,
	}
	return packages.Load(cfg, patterns...)
}

// RunGoTestsInVM compiles the tests found in pkgs and runs them in a QEMU VM
// configured in options `o`. It collects the test results and provides a
// pass/fail result of each individual test.
//
// RunGoTestsInVM runs tests and benchmarks, but not fuzz tests. Guest test
// architecture can be set with VMTEST_ARCH.
//
// The test environment in the VM is very minimal. If a test depends on other
// binaries or specific files to be present, they must be specified with
// additional initramfs commands via WithMergedInitramfs.
//
// All files and directories in the same directory as the test package will be
// made available to the test in the guest as well (e.g. testdata/
// directories).
//
// Coverage from the Go tests is collected if a coverage file name is specified
// via the VMTEST_GO_PROFILE env var.
//
//   - TODO: specify test, bench, fuzz filter. Flags for fuzzing.
//   - TODO: specify timeouts for individual tests.
//   - TODO: check each test's exit status.
func RunGoTestsInVM(t *testing.T, pkgs []string, o ...Opt) {
	SkipWithoutQEMU(t)

	sharedDir := testtmp.TempDir(t)
	vmCoverProfile, ok := os.LookupEnv("VMTEST_GO_PROFILE")
	if !ok {
		t.Log("In-guest Go test coverage is not collected unless VMTEST_GO_PROFILE is set")
	}

	// Set up u-root build options.
	env := golang.Default(golang.DisableCGO(), golang.WithGOARCH(string(qemu.GuestArch().Arch())))

	// Statically build tests and add them to the temporary directory.
	testDir := filepath.Join(sharedDir, "tests")

	if len(vmCoverProfile) > 0 {
		f, err := os.Create(filepath.Join(sharedDir, "coverage.profile"))
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
			"-gcflags=all=-l",
			"-ldflags", "-s -w",
			"-c", pkg,
			"-o", testFile,
		}
		if len(vmCoverProfile) > 0 {
			args = append(args, "-covermode=atomic")
		}
		cmd := env.GoCmd("test", args...)
		if stderr, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("could not build %s: %v\n%s", pkg, err, string(stderr))
		}

		// When a package does not contain any tests, the test
		// executable is not generated, so it is not included in the
		// `tests` list.
		if _, err := os.Stat(testFile); !os.IsNotExist(err) {
			pkgs, err := lookupPkgs(*env, "", pkg)
			if err != nil {
				t.Fatalf("Failed to look up package %q: %v", pkg, err)
			}

			// One directory = one package in standard Go, so
			// finding the first file's parent directory should
			// find us the package directory.
			var dir string
			for _, p := range pkgs {
				if len(p.GoFiles) > 0 {
					dir = filepath.Dir(p.GoFiles[0])
				}
			}
			if dir == "" {
				t.Fatalf("Could not find package directory for %q", pkg)
			}

			// Optimistically copy any files in the pkg's
			// directory, in case e.g. a testdata dir is there.
			if err := copyRelativeFiles(dir, filepath.Join(testDir, pkg)); err != nil {
				t.Fatal(err)
			}
		}
	}

	var uinitArgs []string
	if len(vmCoverProfile) > 0 {
		uinitArgs = append(uinitArgs, "-coverprofile=/gotestdata/coverage.profile")
	}
	initramfs := uroot.Opts{
		Env: env,
		Commands: append(
			uroot.BusyBoxCmds(
				"github.com/u-root/u-root/cmds/core/dhclient",
				"github.com/u-root/u-root/cmds/core/init",
				"github.com/hugelgupf/vmtest/vminit/gouinit",
			),
			uroot.BinaryCmds("cmd/test2json")...),
		InitCmd:   "init",
		UinitCmd:  "gouinit",
		UinitArgs: uinitArgs,
		TempDir:   testtmp.TempDir(t),
	}
	tc := json2test.NewTestCollector()

	// Create the initramfs and start the VM.
	vm := StartVM(t, append(
		[]Opt{
			WithMergedInitramfs(initramfs),
			WithQEMUFn(
				qemu.EventChannelCallback[json2test.TestEvent]("go-test-results", tc.Handle),
				qemu.P9Directory(sharedDir, "gotests"),
			),
			CollectKernelCoverage(),
		}, o...)...)

	if _, err := vm.Console.ExpectString("TESTS PASSED MARKER"); err != nil {
		t.Errorf("Waiting for 'TESTS PASSED MARKER' signal: %v", err)
	}

	if err := vm.Wait(); err != nil {
		t.Errorf("VM exited with %v", err)
	}

	// Collect Go coverage.
	if len(vmCoverProfile) > 0 {
		cov, err := os.Open(filepath.Join(sharedDir, "coverage.profile"))
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

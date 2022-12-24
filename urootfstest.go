// Copyright 2018 the u-root Authors. All rights reserved
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package vmtest

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/hugelgupf/vmtest/qemu"
	"github.com/u-root/u-root/pkg/cp"
	"github.com/u-root/u-root/pkg/golang"
	"github.com/u-root/u-root/pkg/ulog"
	"github.com/u-root/u-root/pkg/ulog/ulogtest"
	"github.com/u-root/u-root/pkg/uroot"
	"github.com/u-root/u-root/pkg/uroot/initramfs"
)

// StartVMTestVM starts u-root-based vmtest VMs that conform to vmtest's
// features and use vmtest's vminit & test framework.
//
// They support:
// - kernel coverage,
// - TODO: tests passed marker.
// - TODO: checking exit status of tests in VM.
func StartVMTestVM(t testing.TB, o *UrootFSOptions) *qemu.VM {
	// Delete any previous coverage data.
	if _, ok := instance[t.Name()]; !ok {
		testCoveragePath := filepath.Join(coveragePath, t.Name())
		if err := os.RemoveAll(testCoveragePath); err != nil && !os.IsNotExist(err) {
			t.Logf("Error erasing previous coverage: %v", err)
		}
	}

	t.Cleanup(func() {
		if err := saveCoverage(t, filepath.Join(o.SharedDir, "kernel_coverage.tar")); err != nil {
			t.Logf("Error saving kernel coverage: %v", err)
		}
	})
	return StartUrootFSVM(t, o)
}

// UrootFSOptions configures a QEMU VM integration test that uses an
// automatically built u-root initramfs as the root file system.
type UrootFSOptions struct {
	// Options are VM configuration options.
	VMOptions

	// BuildOpts are u-root initramfs build options.
	//
	// They are used if the test needs to generate an initramfs.
	// Fields that are not set are populated as possible.
	BuildOpts uroot.Opts

	// DontSetEnv doesn't set the BuildOpts.Env and uses the user-supplied one.
	//
	// HACK HACK HACK
	//
	// TODO: make uroot.Opts.Env a pointer?
	DontSetEnv bool

	// Logger logs build statements.
	//
	// If unset, an implementation that logs to t.Logf is used.
	Logger ulog.Logger
}

// StartUrootFSVM creates a u-root initramfs with the given options and starts
// a QEMU VM with the created u-root file system.
//
// It uses a caller-supplied kernel, or if not set, one supplied by the
// VMTEST_KERNEL env var.
//
// If VMTEST_INITRAMFS is set, that initramfs overrides the options set in this
// test. (This can be used to, for example, run the same test with an initramfs
// built by bazel rules.)
//
// Automatically sets VMTEST_QEMU_ARCH based on the VMTEST_GOARCH (which is
// runtime.GOARCH by default).
func StartUrootFSVM(t testing.TB, o *UrootFSOptions) *qemu.VM {
	SkipWithoutQEMU(t)

	if len(o.Name) == 0 {
		o.Name = t.Name()
	}
	if o.Logger == nil {
		o.Logger = &ulogtest.Logger{TB: t}
	}
	if o.SharedDir == "" {
		o.SharedDir = t.TempDir()
	}

	os.Setenv("VMTEST_QEMU_ARCH", qemu.GOARCHToQEMUArch[GoTestArch()])

	// Set the initramfs.
	if len(o.VMOptions.QEMUOpts.Initramfs) == 0 {
		o.VMOptions.QEMUOpts.Initramfs = filepath.Join(o.SharedDir, "initramfs.cpio")
		if err := ChooseTestInitramfs(o.Logger, o.DontSetEnv, o.BuildOpts, o.VMOptions.QEMUOpts.Initramfs); err != nil {
			t.Fatalf("Could not choose an initramfs for u-root-initramfs-based VM test: %v", err)
		}
	}

	return StartVM(t, &o.VMOptions)
}

// Tests are run from u-root/integration/{gotests,generic-tests}/
const coveragePath = "../coverage"

// Keeps track of the number of instances per test so we do not overlap
// coverage reports.
var instance = map[string]int{}

func saveCoverage(t testing.TB, path string) error {
	// Coverage may not have been collected, for example if the kernel is
	// not built with CONFIG_GCOV_KERNEL.
	if fi, err := os.Stat(path); os.IsNotExist(err) || (err != nil && !fi.Mode().IsRegular()) {
		return nil
	}

	// Move coverage to common directory.
	uniqueCoveragePath := filepath.Join(coveragePath, t.Name(), fmt.Sprintf("%d", instance[t.Name()]))
	instance[t.Name()]++
	if err := os.MkdirAll(uniqueCoveragePath, 0o770); err != nil {
		return err
	}
	if err := os.Rename(path, filepath.Join(uniqueCoveragePath, filepath.Base(path))); err != nil {
		return err
	}
	return nil
}

// ChooseTestInitramfs chooses which initramfs will be used for a given test and
// places it at the location given by outputFile.
// Default to the override initramfs if one is specified in the UROOT_INITRAMFS
// environment variable. Else, build an initramfs with the given parameters.
// If no uinit was provided, the generic one is used.
func ChooseTestInitramfs(logger ulog.Logger, dontSetEnv bool, o uroot.Opts, outputFile string) error {
	override := os.Getenv("VMTEST_INITRAMFS")
	if len(override) > 0 {
		log.Printf("Overriding with initramfs %q from VMTEST_INITRAMFS", override)
		return cp.Copy(override, outputFile)
	}

	_, err := CreateTestInitramfs(logger, dontSetEnv, o, outputFile)
	return err
}

// GoTestArch returns the architecture under test. Pass this as GOARCH when
// building Go programs to be run in the QEMU environment.
func GoTestArch() string {
	if env := os.Getenv("VMTEST_GOARCH"); env != "" {
		return env
	}
	return runtime.GOARCH
}

// CreateTestInitramfs creates an initramfs with the given build options
// and writes it to the given output file. If no output file is provided,
// one will be created.
// The output file name is returned. It is the caller's responsibility to remove
// the initramfs file after use.
func CreateTestInitramfs(logger ulog.Logger, dontSetEnv bool, o uroot.Opts, outputFile string) (string, error) {
	if !dontSetEnv {
		env := golang.Default()
		env.CgoEnabled = false
		env.GOARCH = GoTestArch()
		o.Env = env
	}

	// If build opts don't specify any commands, include all commands. Else,
	// always add init and elvish.
	o.AddBusyBoxCommands(
		"github.com/u-root/u-root/cmds/core/init",
		"github.com/u-root/u-root/cmds/core/elvish",
	)

	// Fill in the default build options if not specified.
	if o.BaseArchive == nil {
		o.BaseArchive = uroot.DefaultRamfs().Reader()
	}
	if len(o.InitCmd) == 0 {
		o.InitCmd = "init"
	}
	if len(o.DefaultShell) == 0 {
		o.DefaultShell = "elvish"
	}
	if len(o.TempDir) == 0 {
		tempDir, err := os.MkdirTemp("", "initramfs-tempdir")
		if err != nil {
			return "", fmt.Errorf("Failed to create temp dir: %v", err)
		}
		defer os.RemoveAll(tempDir)
		o.TempDir = tempDir
	}

	// Create an output file if one was not provided.
	if len(outputFile) == 0 {
		f, err := os.CreateTemp("", "initramfs.cpio")
		if err != nil {
			return "", fmt.Errorf("failed to create output file: %v", err)
		}
		outputFile = f.Name()
	}
	w, err := initramfs.CPIO.OpenWriter(logger, outputFile)
	if err != nil {
		return "", fmt.Errorf("failed to create initramfs writer: %v", err)
	}
	o.OutputFile = w

	if err := uroot.CreateInitramfs(logger, o); err != nil {
		return "", fmt.Errorf("error creating initramfs: %v", err)
	}
	return outputFile, nil
}

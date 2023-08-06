// Copyright 2022 the u-root Authors. All rights reserved
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package vmtest

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/hugelgupf/vmtest/qemu"
	"github.com/hugelgupf/vmtest/uqemu"
	"github.com/u-root/u-root/pkg/ulog"
	"github.com/u-root/u-root/pkg/ulog/ulogtest"
	"github.com/u-root/u-root/pkg/uroot"
)

// startVMTestVM starts u-root-based vmtest VMs that conform to vmtest's
// features and use vmtest's vminit & test framework.
//
// They support:
//
//   - kernel coverage,
//   - TODO: tests passed marker.
//   - TODO: checking exit status of tests in VM.
func startVMTestVM(t testing.TB, o *UrootFSOptions) *qemu.VM {
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
// If VMTEST_INITRAMFS_OVERRIDE is set, that initramfs overrides the options
// set in this test. (This can be used to, for example, run the same test with
// an initramfs built by bazel rules.)
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

	qemuOpts := o.getQEMUOpts(t)
	vmopts := o.VMOptions
	vmopts.QEMUOpts = *qemuOpts

	return StartVM(t, &vmopts)
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

func (o *UrootFSOptions) getQEMUOpts(t testing.TB) *qemu.Options {
	// Always add init and elvish.
	o.BuildOpts.AddBusyBoxCommands(
		"github.com/u-root/u-root/cmds/core/init",
		"github.com/u-root/u-root/cmds/core/elvish",
	)
	if len(o.BuildOpts.InitCmd) == 0 {
		o.BuildOpts.InitCmd = "init"
	}
	if len(o.BuildOpts.DefaultShell) == 0 {
		o.BuildOpts.DefaultShell = "elvish"
	}
	if len(o.BuildOpts.TempDir) == 0 {
		tempDir := filepath.Join(o.SharedDir, "initramfs-tempdir")
		err := os.Mkdir(tempDir, 0755)
		if err != nil {
			t.Fatalf("Failed to create temp dir: %v", err)
		}
		defer os.RemoveAll(tempDir)

		o.BuildOpts.TempDir = tempDir
	}

	uopts := uqemu.Options{
		Initramfs:  o.BuildOpts,
		VMOpts:     o.VMOptions.QEMUOpts,
		InitrdPath: filepath.Join(o.SharedDir, "initramfs.cpio"),
	}
	qemuOpts, err := uopts.BuildInitramfs(o.Logger)
	if err != nil {
		t.Fatalf("Failed to build an initramfs: %v", err)
	}
	return qemuOpts
}

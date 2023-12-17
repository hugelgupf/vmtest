// Copyright 2022 the u-root Authors. All rights reserved
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package vmtest

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

// startVMTestVM starts u-root-based vmtest VMs that conform to vmtest's
// features and use vmtest's vminit & test framework.
//
// They support:
//
//   - kernel coverage,
//   - TODO: tests passed marker.
//   - TODO: checking exit status of tests in VM.
/*func startVMTestVM(t testing.TB, o *UrootFSOptions) *qemu.VM {
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
}*/

// Tests are run from u-root/integration/{gotests,generic-tests}/.
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
	return os.Rename(path, filepath.Join(uniqueCoveragePath, filepath.Base(path)))
}

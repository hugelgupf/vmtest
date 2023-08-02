// Copyright 2022 the u-root Authors. All rights reserved
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package guest has functions for use in tests running in VM guests.
package guest

import (
	"testing"

	"github.com/u-root/u-root/pkg/cmdline"
)

// SkipIfNotInVM skips the test if it is not running in a vmtest-started VM.
//
// The presence of "uroot.vmtest" on the kernel commandline is used to
// determine this.
func SkipIfNotInVM(t testing.TB) {
	if !cmdline.ContainsFlag("uroot.vmtest") {
		t.Skip("Skipping test -- must be run inside vmtest VM")
	}
}

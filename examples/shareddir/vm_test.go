// Copyright 2023 the u-root Authors. All rights reserved
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package shareddir_x_test

import (
	"testing"

	"github.com/hugelgupf/vmtest/qemu"
	"github.com/hugelgupf/vmtest/qemu/quimage"
	"github.com/u-root/mkuimage/uimage"
)

func TestMount(t *testing.T) {
	vm := qemu.StartT(
		t,
		"vm",
		qemu.ArchUseEnvv,
		quimage.WithUimageT(t,
			uimage.WithInit("init"),
			uimage.WithUinit("shutdownafter", "--", "vmmount", "--", "cat", "/mount/9p/vmtestdir/LICENSE"),
			uimage.WithBusyboxCommands(
				"github.com/u-root/u-root/cmds/core/init",
				"github.com/u-root/u-root/cmds/core/cat",
				"github.com/hugelgupf/vmtest/vminit/vmmount",
				"github.com/hugelgupf/vmtest/vminit/shutdownafter",
			),
		),
		qemu.P9Directory("../../", "vmtestdir"),
	)

	if _, err := vm.Console.ExpectString("BSD 3-Clause License"); err != nil {
		t.Errorf("Did not see LICENSE: %v", err)
	}

	if err := vm.Wait(); err != nil {
		t.Fatalf("Error waiting for VM to exit: %v", err)
	}
}

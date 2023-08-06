// Copyright 2023 the u-root Authors. All rights reserved
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package uqemu

import (
	"bufio"
	"io"
	"path/filepath"
	"sync"
	"testing"

	"github.com/hugelgupf/vmtest/qemu"
	"github.com/u-root/u-root/pkg/ulog/ulogtest"
	"github.com/u-root/u-root/pkg/uroot"
)

func replaceCtl(str []byte) []byte {
	for i, c := range str {
		if c == 9 || c == 10 {
		} else if c < 32 || c == 127 {
			str[i] = '~'
		}
	}
	return str
}

func TestStartVM(t *testing.T) {
	tmp := t.TempDir()
	logger := &ulogtest.Logger{TB: t}
	initrdPath := filepath.Join(tmp, "initramfs.cpio")

	r, w := io.Pipe()
	var kernelArgs string
	switch GuestGOARCH() {
	case "arm":
		kernelArgs = "console=ttyAMA0"
	case "amd64":
		kernelArgs = "console=ttyS0 earlyprintk=ttyS0"
	}
	uopts := Options{
		Initramfs: uroot.Opts{
			InitCmd:  "init",
			UinitCmd: "qemutest1",
			TempDir:  tmp,
			Commands: uroot.BusyBoxCmds(
				"github.com/u-root/u-root/cmds/core/init",
				"github.com/hugelgupf/vmtest/qemu/qemutest1",
			),
		},
		InitrdPath: initrdPath,
		VMOpts: qemu.Options{
			SerialOutput: w,
			KernelArgs:   kernelArgs,
		},
	}

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		s := bufio.NewScanner(r)
		for s.Scan() {
			t.Logf("vm: %s", replaceCtl(s.Bytes()))
		}
		if err := s.Err(); err != nil {
			t.Errorf("Error reading serial from VM: %v", err)
		}
	}()

	vm, err := uopts.Start(logger)
	if err != nil {
		t.Fatalf("Failed to start VM: %v", err)
	}
	t.Logf("cmdline: %#v", vm.CmdlineQuoted())

	if _, err := vm.Console.ExpectString("I AM HERE"); err != nil {
		t.Errorf("Error expecting I AM HERE: %v", err)
	}

	if err := vm.Wait(); err != nil {
		t.Fatalf("Error waiting for VM to exit: %v", err)
	}
	wg.Wait()
}

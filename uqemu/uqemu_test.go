// Copyright 2023 the u-root Authors. All rights reserved
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package uqemu

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"

	"github.com/hugelgupf/vmtest/qemu"
	"github.com/u-root/gobusybox/src/pkg/golang"
	"github.com/u-root/u-root/pkg/ulog/ulogtest"
	"github.com/u-root/u-root/pkg/uroot"
)

func TestOverride(t *testing.T) {
	resetVars := []string{
		"VMTEST_QEMU",
		"VMTEST_QEMU_ARCH",
		"VMTEST_GOARCH",
		"VMTEST_KERNEL",
		"VMTEST_INITRAMFS",
		"VMTEST_INITRAMFS_OVERRIDE",
	}
	// In case these env vars are actually set by calling env & used below
	// in other tests, save their values, set them to empty for duration of
	// test & restore them after.
	savedEnv := make(map[string]string)
	for _, key := range resetVars {
		savedEnv[key] = os.Getenv(key)
		os.Setenv(key, "")
	}
	t.Cleanup(func() {
		for key, val := range savedEnv {
			os.Setenv(key, val)
		}
	})

	for _, tt := range []struct {
		name          string
		envv          map[string]string
		o             *options
		wantInitramfs string
		wantArch      qemu.GuestArch
		err           error
	}{
		{
			name: "initramfs-override",
			envv: map[string]string{
				"VMTEST_INITRAMFS_OVERRIDE": "./foo.cpio",
				"VMTEST_GOARCH":             "amd64",
			},
			o: &options{
				initramfs: uroot.Opts{Env: golang.Default(golang.WithGOARCH("386"))},
			},
			wantInitramfs: "./foo.cpio",
			wantArch:      qemu.GuestArchI386,
		},
		{
			name: "initramfs-override-and-goarch",
			envv: map[string]string{
				"VMTEST_INITRAMFS_OVERRIDE": "./foo.cpio",
				"VMTEST_GOARCH":             "386",
			},
			o:             &options{},
			wantInitramfs: "./foo.cpio",
			wantArch:      qemu.GuestArchI386,
		},
		{
			name: "initramfs-override-and-runtime-goarch",
			envv: map[string]string{
				"VMTEST_INITRAMFS_OVERRIDE": "./foo.cpio",
			},
			o:             &options{},
			wantInitramfs: "./foo.cpio",
			wantArch:      qemu.GuestArchX8664,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			for key, val := range tt.envv {
				os.Setenv(key, val)
			}
			t.Cleanup(func() {
				for key := range tt.envv {
					os.Setenv(key, "")
				}
			})

			got, err := qemu.OptionsFor(tt.o)
			if !errors.Is(err, tt.err) {
				t.Errorf("Build = %v, want %v", err, tt.err)
			}
			if got.Initramfs != tt.wantInitramfs {
				t.Errorf("Build = %v, want %v", got.Initramfs, tt.wantInitramfs)
			}
			if arch := got.GuestArch(); arch != tt.wantArch {
				t.Errorf("Build = arch %v, want arch %v", arch, tt.wantArch)
			}
		})
	}
}

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

	initramfs := uroot.Opts{
		InitCmd:  "init",
		UinitCmd: "qemutest1",
		TempDir:  tmp,
		Commands: uroot.BusyBoxCmds(
			"github.com/u-root/u-root/cmds/core/init",
			"github.com/hugelgupf/vmtest/qemu/test/qemutest1",
		),
	}
	vm, err := qemu.Start(WithUrootInitramfs(logger, initramfs, initrdPath), qemu.WithSerialOutput(w))
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

func TestTask(t *testing.T) {
	tmp := t.TempDir()
	logger := &ulogtest.Logger{TB: t}
	initrdPath := filepath.Join(tmp, "initramfs.cpio")

	r, w := io.Pipe()

	var taskGotCanceled bool
	var taskSawIAmHere bool
	var vmExitErr error

	initramfs := uroot.Opts{
		InitCmd:  "init",
		UinitCmd: "qemutest1",
		TempDir:  tmp,
		Commands: uroot.BusyBoxCmds(
			"github.com/u-root/u-root/cmds/core/init",
			"github.com/hugelgupf/vmtest/qemu/test/qemutest1",
		),
	}
	vm, err := qemu.Start(
		WithUrootInitramfs(logger, initramfs, initrdPath),
		qemu.WithSerialOutput(w),
		// Tests that we can wait for VM to start.
		qemu.WithTask(qemu.WaitVMStarted(func(ctx context.Context, n *qemu.Notifications) error {
			s := bufio.NewScanner(r)
			for s.Scan() {
				line := string(replaceCtl(s.Bytes()))
				if strings.Contains(line, "I AM HERE") {
					taskSawIAmHere = true
				}
				t.Logf("vm: %s", line)
			}
			if err := s.Err(); err != nil {
				return fmt.Errorf("error reading serial from VM: %v", err)
			}
			return nil
		})),

		// Make sure that the test does not time out
		// forever -- context must get canceled.
		qemu.WithTask(func(ctx context.Context, n *qemu.Notifications) error {
			<-ctx.Done()
			taskGotCanceled = true
			return nil
		}),

		// Make sure that the VMExit event is always there.
		qemu.WithTask(func(ctx context.Context, n *qemu.Notifications) error {
			err, ok := <-n.VMExited
			if !ok {
				return fmt.Errorf("failed to read from VM exit notifications")
			}
			vmExitErr = err
			return nil
		}),
	)
	if err != nil {
		t.Fatalf("Failed to start VM: %v", err)
	}
	t.Logf("cmdline: %#v", vm.CmdlineQuoted())

	if _, err := vm.Console.ExpectString("I AM HERE"); err != nil {
		t.Errorf("Error expecting I AM HERE: %v", err)
	}

	werr := vm.Wait()
	if werr != nil {
		t.Errorf("Error waiting for VM to exit: %v", werr)
	}

	if !reflect.DeepEqual(werr, vmExitErr) {
		t.Errorf("Error: Exit notification error is %v, want %v", vmExitErr, werr)
	}
	if !taskGotCanceled {
		t.Error("Error: Task did not get canceled")
	}
	if !taskSawIAmHere {
		t.Error("Error: Serial console task didn't see I AM HERE")
	}
}

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
	"github.com/hugelgupf/vmtest/qemu/test/eventemitter/event"
	"github.com/u-root/u-root/pkg/ulog/ulogtest"
	"github.com/u-root/u-root/pkg/uroot"
)

func TestOverride(t *testing.T) {
	t.Cleanup(func() {
		os.Setenv("VMTEST_INITRAMFS_OVERRIDE", "")
	})

	want := "foo.cpio"
	os.Setenv("VMTEST_INITRAMFS_OVERRIDE", "foo.cpio")

	got, err := qemu.OptionsFor(qemu.ArchUseEnvv, WithUrootInitramfs(nil, uroot.Opts{}, ""))
	if err != nil {
		t.Errorf("OptionsFor = %v", err)
	}
	if got.Initramfs != want {
		t.Errorf("Initramfs = %v, want %v", got.Initramfs, want)
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
	vm, err := qemu.Start(qemu.ArchUseEnvv, WithUrootInitramfs(logger, initramfs, initrdPath), qemu.WithSerialOutput(w))
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
		qemu.ArchUseEnvv,
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

func TestEventChannel(t *testing.T) {
	logger := &ulogtest.Logger{TB: t}

	initramfs := uroot.Opts{
		InitCmd:  "init",
		UinitCmd: "eventemitter",
		TempDir:  t.TempDir(),
		Commands: uroot.BusyBoxCmds(
			"github.com/u-root/u-root/cmds/core/init",
			"github.com/hugelgupf/vmtest/qemu/test/eventemitter",
		),
	}
	events := make(chan event.Event)
	vm, err := qemu.Start(
		qemu.ArchUseEnvv,
		WithUrootInitramfs(logger, initramfs, filepath.Join(t.TempDir(), "initramfs.cpio")),
		qemu.LogSerialByLine(qemu.PrintLineWithPrefix("vm", t.Logf)),
		qemu.EventChannel[event.Event]("test", events),
	)
	if err != nil {
		t.Fatalf("Failed to start VM: %v", err)
	}
	t.Logf("cmdline: %#v", vm.CmdlineQuoted())

	// Expect event IDs 0 through 999, in order.
	i := 0
	for e := range events {
		if e.ID != i {
			t.Errorf("The %dth event has ID %d, want %d", i+1, e.ID, i)
		}
		i++
	}
	if i != 1000 {
		t.Errorf("Expected last event ID to be 1000, got %d", i)
	}

	if _, err := vm.Console.ExpectString("TEST PASSED"); err != nil {
		t.Errorf("Error expecting TEST PASSED: %v", err)
	}

	if err := vm.Wait(); err != nil {
		t.Fatalf("Error waiting for VM to exit: %v", err)
	}
}

func TestEventChannelErrorWithoutDoneEvent(t *testing.T) {
	logger := &ulogtest.Logger{TB: t}

	initramfs := uroot.Opts{
		InitCmd:  "init",
		UinitCmd: "eventemitter",
		// Instruct eventemitter not to close the event channel.
		UinitArgs: []string{"-skip-close"},
		TempDir:   t.TempDir(),
		Commands: uroot.BusyBoxCmds(
			"github.com/u-root/u-root/cmds/core/init",
			"github.com/hugelgupf/vmtest/qemu/test/eventemitter",
		),
	}
	events := make(chan event.Event)
	vm, err := qemu.Start(
		qemu.ArchUseEnvv,
		WithUrootInitramfs(logger, initramfs, filepath.Join(t.TempDir(), "initramfs.cpio")),
		qemu.LogSerialByLine(qemu.PrintLineWithPrefix("vm", t.Logf)),
		qemu.EventChannel[event.Event]("test", events),
	)
	if err != nil {
		t.Fatalf("Failed to start VM: %v", err)
	}
	t.Logf("cmdline: %#v", vm.CmdlineQuoted())

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		// Drain the events.
		for range events {
		}
		wg.Done()
	}()

	want := qemu.ErrEventChannelMissingDoneEvent
	if err := vm.Wait(); !errors.Is(err, want) {
		t.Fatalf("VM.Wait =  %v, want %v", err, want)
	}
	// Ensure that event channel is closed even in error case.
	wg.Wait()
}

// Tests that we do not hang forever when HaltOnKernelPanic is passed.
func TestKernelPanic(t *testing.T) {
	logger := &ulogtest.Logger{TB: t}

	// init exits after not finding anything to do, so kernel panics.
	initramfs := uroot.Opts{
		InitCmd: "init",
		TempDir: t.TempDir(),
		Commands: uroot.BusyBoxCmds(
			"github.com/u-root/u-root/cmds/core/init",
		),
	}

	vm, err := qemu.Start(
		qemu.ArchUseEnvv,
		WithUrootInitramfs(logger, initramfs, filepath.Join(t.TempDir(), "initramfs.cpio")),
		qemu.LogSerialByLine(qemu.PrintLineWithPrefix("vm", t.Logf)),
		qemu.HaltOnKernelPanic(),
	)
	if err != nil {
		t.Fatalf("Failed to start VM: %v", err)
	}
	t.Logf("cmdline: %#v", vm.CmdlineQuoted())

	if _, err := vm.Console.ExpectString("Kernel panic"); err != nil {
		t.Errorf("Expect(Kernel panic) = %v", err)
	}

	if err := vm.Wait(); err != nil {
		t.Fatalf("VM.Wait = %v", err)
	}
}

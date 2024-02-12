// Copyright 2018 the u-root Authors. All rights reserved
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package qevent

import (
	"errors"
	"path/filepath"
	"sync"
	"testing"

	"github.com/hugelgupf/vmtest/qemu"
	"github.com/hugelgupf/vmtest/tests/cmds/eventemitter/event"
	"github.com/hugelgupf/vmtest/uqemu"
	"github.com/u-root/u-root/pkg/uroot"
	"golang.org/x/sys/unix"
)

func TestEventChannel(t *testing.T) {
	initramfs := uroot.Opts{
		InitCmd:  "init",
		UinitCmd: "eventemitter",
		Commands: uroot.BusyBoxCmds(
			"github.com/u-root/u-root/cmds/core/init",
			"github.com/hugelgupf/vmtest/tests/cmds/eventemitter",
		),
	}
	events := make(chan event.Event)
	vm, err := qemu.Start(
		qemu.ArchUseEnvv,
		uqemu.WithUrootInitramfsT(t, initramfs),
		qemu.LogSerialByLine(qemu.DefaultPrint("vm", t.Logf)),
		EventChannel[event.Event]("test", events),
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
	initramfs := uroot.Opts{
		InitCmd:  "init",
		UinitCmd: "eventemitter",
		// Instruct eventemitter not to close the event channel.
		UinitArgs: []string{"-skip-close"},
		Commands: uroot.BusyBoxCmds(
			"github.com/u-root/u-root/cmds/core/init",
			"github.com/hugelgupf/vmtest/tests/cmds/eventemitter",
		),
	}
	events := make(chan event.Event)
	vm, err := qemu.Start(
		qemu.ArchUseEnvv,
		uqemu.WithUrootInitramfsT(t, initramfs),
		qemu.LogSerialByLine(qemu.DefaultPrint("vm", t.Logf)),
		EventChannel[event.Event]("test", events),
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

	want := ErrEventChannelMissingDoneEvent
	if err := vm.Wait(); !errors.Is(err, want) {
		t.Fatalf("VM.Wait =  %v, want %v", err, want)
	}
	// Ensure that event channel is closed even in error case.
	wg.Wait()
}

func TestEventChannelCallback(t *testing.T) {
	initramfs := uroot.Opts{
		InitCmd:  "init",
		UinitCmd: "eventemitter",
		Commands: uroot.BusyBoxCmds(
			"github.com/u-root/u-root/cmds/core/init",
			"github.com/hugelgupf/vmtest/tests/cmds/eventemitter",
		),
	}
	var events []event.Event
	vm, err := qemu.Start(
		qemu.ArchUseEnvv,
		uqemu.WithUrootInitramfsT(t, initramfs),
		qemu.LogSerialByLine(qemu.DefaultPrint("vm", t.Logf)),
		EventChannelCallback[event.Event]("test", func(e event.Event) {
			events = append(events, e)
		}),
	)
	if err != nil {
		t.Fatalf("Failed to start VM: %v", err)
	}
	t.Logf("cmdline: %#v", vm.CmdlineQuoted())

	if _, err := vm.Console.ExpectString("TEST PASSED"); err != nil {
		t.Errorf("Error expecting TEST PASSED: %v", err)
	}

	if err := vm.Wait(); err != nil {
		t.Fatalf("Error waiting for VM to exit: %v", err)
	}

	// Expect event IDs 0 through 999, in order.
	i := 0
	for _, e := range events {
		if e.ID != i {
			t.Errorf("The %dth event has ID %d, want %d", i+1, e.ID, i)
		}
		i++
	}
	if i != 1000 {
		t.Errorf("Expected last event ID to be 1000, got %d", i)
	}
}

func TestEventChannelCallbackDoesNotHang(t *testing.T) {
	_, err := qemu.Start(qemu.ArchAMD64,
		// Some path that does not exist.
		qemu.WithQEMUCommand(filepath.Join(t.TempDir(), "qemu")),

		// Make sure this doesn't hang if process is never started.
		EventChannelCallback[event.Event]("test", func(e event.Event) {}),
	)

	if !errors.Is(err, unix.ENOENT) {
		t.Fatalf("Failed to start VM: %v", err)
	}
}

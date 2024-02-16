// Copyright 2023 the u-root Authors. All rights reserved
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package quimage

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/hugelgupf/vmtest/qemu"
	"github.com/u-root/mkuimage/uimage"
	"github.com/u-root/uio/llog"
)

func TestOverride(t *testing.T) {
	want := "foo.cpio"
	t.Setenv("VMTEST_INITRAMFS_OVERRIDE", "foo.cpio")

	got, err := qemu.OptionsFor(qemu.ArchUseEnvv, WithUimage(nil, ""))
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
	vm := qemu.StartT(
		t,
		"vm",
		qemu.ArchUseEnvv,
		WithUimageT(t,
			uimage.WithInit("init"),
			uimage.WithUinit("helloworld"),
			uimage.WithBusyboxCommands(
				"github.com/u-root/u-root/cmds/core/init",
				"github.com/hugelgupf/vmtest/tests/cmds/helloworld",
			),
		),
	)

	if _, err := vm.Console.ExpectString("Hello world"); err != nil {
		t.Errorf("Error expecting Hello world: %v", err)
	}

	if err := vm.Wait(); err != nil {
		t.Fatalf("Error waiting for VM to exit: %v", err)
	}
}

func TestTask(t *testing.T) {
	r, w := io.Pipe()

	var taskGotCanceled bool
	var taskSawHelloWorld bool
	var vmExitErr error

	vm, err := qemu.Start(
		qemu.ArchUseEnvv,
		WithUimageT(t,
			uimage.WithInit("init"),
			uimage.WithUinit("helloworld"),
			uimage.WithBusyboxCommands(
				"github.com/u-root/u-root/cmds/core/init",
				"github.com/hugelgupf/vmtest/tests/cmds/helloworld",
			),
		),
		qemu.WithSerialOutput(w),
		// Tests that we can wait for VM to start.
		qemu.WithTask(qemu.WaitVMStarted(func(ctx context.Context, n *qemu.Notifications) error {
			s := bufio.NewScanner(r)
			for s.Scan() {
				line := string(replaceCtl(s.Bytes()))
				if strings.Contains(line, "Hello world") {
					taskSawHelloWorld = true
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

	if _, err := vm.Console.ExpectString("Hello world"); err != nil {
		t.Errorf("Error expecting Hello world: %v", err)
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
	if !taskSawHelloWorld {
		t.Error("Error: Serial console task didn't see Hello world")
	}
}

// Tests that we do not hang forever when HaltOnKernelPanic is passed.
func TestKernelPanic(t *testing.T) {
	// init exits after not finding anything to do, so kernel panics.
	vm := qemu.StartT(
		t,
		"vm",
		qemu.ArchUseEnvv,
		WithUimageT(t,
			uimage.WithInit("init"),
			uimage.WithBusyboxCommands(
				"github.com/u-root/u-root/cmds/core/init",
			),
		),
		qemu.HaltOnKernelPanic(),
	)

	if _, err := vm.Console.ExpectString("Kernel panic"); err != nil {
		t.Errorf("Expect(Kernel panic) = %v", err)
	}

	if err := vm.Wait(); err != nil {
		t.Fatalf("VM.Wait = %v", err)
	}
}

func TestInvalidInitramfs(t *testing.T) {
	logger := llog.Test(t)

	for _, tt := range []struct {
		name       string
		initramfs  []uimage.Modifier
		initrdPath string
	}{
		{
			name: "missing-tmpdir",
			initramfs: []uimage.Modifier{
				uimage.WithInit("init"),
				uimage.WithUinit("qemutest1"),
				uimage.WithBusyboxCommands(
					"github.com/u-root/u-root/cmds/core/init",
					"github.com/hugelgupf/vmtest/tests/cmds/qemutest1",
				),
			},
			initrdPath: filepath.Join(t.TempDir(), "initramfs.cpio"),
		},
		{
			name:       "output-path-is-dir",
			initrdPath: t.TempDir(),
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			_, err := qemu.Start(qemu.ArchUseEnvv,
				WithUimage(logger, tt.initrdPath, tt.initramfs...),
				qemu.LogSerialByLine(qemu.DefaultPrint("vm", t.Logf)),
			)
			if err == nil {
				t.Fatalf("VM expected error, got nil")
			}
			t.Logf("Error: %v", err)
		})
	}
}

func TestOutputFillsConsoleBuffers(t *testing.T) {
	// 4000 repeats of Hello world fill the buffer of the pty used by the
	// Expect library. Make sure this does not cause hangs.
	vm := qemu.StartT(
		t,
		"vm",
		qemu.ArchUseEnvv,
		WithUimageT(t,
			uimage.WithInit("init"),
			uimage.WithUinit("helloworld", "-n", "4000"),
			uimage.WithBusyboxCommands(
				"github.com/u-root/u-root/cmds/core/init",
				"github.com/hugelgupf/vmtest/tests/cmds/helloworld",
			),
		),
	)

	// No calls to Expect means nothing is draining the Console pty buffer.

	if err := vm.Wait(); err != nil {
		t.Fatalf("Error waiting for VM to exit: %v", err)
	}
}

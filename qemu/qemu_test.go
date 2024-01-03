// Copyright 2018 the u-root Authors. All rights reserved
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package qemu

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	"github.com/u-root/gobusybox/src/pkg/golang"
	"github.com/u-root/u-root/pkg/ulog/ulogtest"
	"github.com/u-root/u-root/pkg/uroot"
	"github.com/u-root/u-root/pkg/uroot/initramfs"
	"golang.org/x/exp/slices"
	"golang.org/x/sys/unix"
)

type cmdlineEqualOpt func(*cmdlineEqualOption)

func withArgv0(argv0 string) func(*cmdlineEqualOption) {
	return func(o *cmdlineEqualOption) {
		o.argv0 = argv0
	}
}

func withArg(arg ...string) func(*cmdlineEqualOption) {
	return func(o *cmdlineEqualOption) {
		o.components = append(o.components, arg)
	}
}

type cmdlineEqualOption struct {
	argv0      string
	components [][]string
}

func isCmdlineEqual(got []string, opts ...cmdlineEqualOpt) error {
	var opt cmdlineEqualOption
	for _, o := range opts {
		o(&opt)
	}

	if len(got) == 0 && len(opt.argv0) == 0 && len(opt.components) == 0 {
		return nil
	}
	if len(got) == 0 {
		return fmt.Errorf("empty cmdline")
	}
	if got[0] != opt.argv0 {
		return fmt.Errorf("argv0 does not match: got %v, want %v", got[0], opt.argv0)
	}
	got = got[1:]
	for _, component := range opt.components {
		found := false
		for i := range got {
			if slices.Compare(got[i:i+len(component)], component) == 0 {
				found = true
				got = slices.Delete(got, i, i+len(component))
				break
			}
		}
		if !found {
			return fmt.Errorf("cmdline component %#v not found", component)
		}
	}
	if len(got) > 0 {
		return fmt.Errorf("extraneous cmdline arguments: %#v", got)
	}
	return nil
}

func TestCmdline(t *testing.T) {
	resetVars := []string{
		"VMTEST_QEMU",
		"VMTEST_QEMU_ARCH",
		"VMTEST_KERNEL",
		"VMTEST_INITRAMFS",
		"VMTEST_TIMEOUT",
	}
	for _, key := range resetVars {
		t.Setenv(key, "")
	}

	for _, tt := range []struct {
		name string

		// Inputs
		arch Arch
		fns  []Fn
		envv map[string]string

		// Outputs
		want        []cmdlineEqualOpt
		wantTimeout time.Duration
		err         error
		cmdlineErr  error
	}{
		{
			name: "simple",
			arch: ArchAMD64,
			fns:  []Fn{WithQEMUCommand("qemu"), WithKernel("./foobar")},
			want: []cmdlineEqualOpt{
				withArgv0("qemu"),
				withArg("-nographic"),
				withArg("-kernel", "./foobar"),
			},
		},
		{
			name: "empty-kernel-and-args",
			arch: ArchAMD64,
			fns: []Fn{
				WithQEMUCommand("qemu"),
				WithKernel(""),
				// Empty should work.
				WithAppendKernel(),
			},
			want: []cmdlineEqualOpt{
				withArgv0("qemu"),
				withArg("-nographic"),
			},
		},
		{
			name: "option-error",
			arch: ArchAMD64,
			fns: []Fn{
				func(_ *IDAllocator, _ *Options) error {
					return ErrKernelRequiredForArgs
				},
			},
			err: ErrKernelRequiredForArgs,
		},
		{
			name: "invalid-arch",
			arch: Arch("amd66"),
			err:  ErrUnsupportedArch,
		},
		{
			name:       "kernel-args-fail",
			arch:       ArchAMD64,
			fns:        []Fn{WithQEMUCommand("qemu"), WithAppendKernel("printk=ttyS0")},
			cmdlineErr: ErrKernelRequiredForArgs,
		},
		{
			name: "kernel-args-initrd-with-precedence-over-env",
			arch: ArchAMD64,
			fns: []Fn{
				WithQEMUCommand("qemu"),
				WithKernel("./foobar"),
				WithInitramfs("./initrd"),
				WithAppendKernel("printk=ttyS0"),
			},
			envv: map[string]string{
				"VMTEST_QEMU":      "qemu-system-x86_64 -enable-kvm -m 1G",
				"VMTEST_QEMU_ARCH": "i386",
				"VMTEST_KERNEL":    "./baz",
				"VMTEST_INITRAMFS": "./init.cpio",
			},
			want: []cmdlineEqualOpt{
				withArgv0("qemu"),
				withArg("-nographic"),
				withArg("-kernel", "./foobar"),
				withArg("-initrd", "./initrd"),
				withArg("-append", "printk=ttyS0"),
			},
		},
		{
			name: "id-allocator",
			arch: ArchAMD64,
			fns: []Fn{
				WithQEMUCommand("qemu"),
				WithKernel("./foobar"),
				IDEBlockDevice("./disk1"),
				IDEBlockDevice("./disk2"),
			},
			want: []cmdlineEqualOpt{
				withArgv0("qemu"),
				withArg("-nographic"),
				withArg("-kernel", "./foobar"),
				withArg("-drive", "file=./disk1,if=none,id=drive0",
					"-device", "ich9-ahci,id=ahci0",
					"-device", "ide-hd,drive=drive0,bus=ahci0.0"),
				withArg("-drive", "file=./disk2,if=none,id=drive1",
					"-device", "ich9-ahci,id=ahci1",
					"-device", "ide-hd,drive=drive1,bus=ahci1.0"),
			},
		},
		{
			name: "env-config",
			arch: ArchUseEnvv,
			envv: map[string]string{
				"VMTEST_QEMU":      "qemu-system-x86_64 -enable-kvm -m 1G",
				"VMTEST_QEMU_ARCH": "x86_64",
				"VMTEST_KERNEL":    "./foobar",
				"VMTEST_INITRAMFS": "./init.cpio",
			},
			want: []cmdlineEqualOpt{
				withArgv0("qemu-system-x86_64"),
				withArg("-nographic"),
				withArg("-enable-kvm", "-m", "1G"),
				withArg("-initrd", "./init.cpio"),
				withArg("-kernel", "./foobar"),
			},
		},
		{
			name: "env-vmtimeout",
			arch: ArchAMD64,
			envv: map[string]string{
				"VMTEST_QEMU":    "qemu",
				"VMTEST_TIMEOUT": "1m30s",
			},
			wantTimeout: 90 * time.Second,
			want: []cmdlineEqualOpt{
				withArgv0("qemu"),
				withArg("-nographic"),
			},
		},
		{
			name: "env-vmtimeout-wrong",
			arch: ArchAMD64,
			envv: map[string]string{
				"VMTEST_TIMEOUT": "900",
			},
			err: ErrInvalidTimeout,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			for key, val := range tt.envv {
				t.Setenv(key, val)
			}
			opts, err := OptionsFor(tt.arch, tt.fns...)
			if !errors.Is(err, tt.err) {
				t.Errorf("Options = %v, want %v", err, tt.err)
			}
			if opts == nil {
				return
			}
			if opts.VMTimeout != tt.wantTimeout {
				t.Errorf("Options.VMTimeout = %s, want %s", opts.VMTimeout, tt.wantTimeout)
			}
			got, err := opts.Cmdline()
			if !errors.Is(err, tt.cmdlineErr) {
				t.Errorf("Cmdline = %v, want %v", err, tt.cmdlineErr)
			}

			t.Logf("Got cmdline: %v", got)
			if err := isCmdlineEqual(got, tt.want...); err != nil {
				t.Errorf("Cmdline = %v", err)
			}
		})
	}
}

func TestStartVM(t *testing.T) {
	tmp := t.TempDir()
	logger := &ulogtest.Logger{TB: t}
	initrdPath := filepath.Join(tmp, "initramfs.cpio")
	initrdWriter, err := initramfs.CPIO.OpenWriter(logger, initrdPath)
	if err != nil {
		t.Fatalf("Failed to create initramfs writer: %v", err)
	}

	env := golang.Default(golang.DisableCGO(), golang.WithGOARCH(string(GuestArch())))

	uopts := uroot.Opts{
		Env:        env,
		InitCmd:    "init",
		UinitCmd:   "qemutest1",
		OutputFile: initrdWriter,
		TempDir:    tmp,
	}
	uopts.AddBusyBoxCommands(
		"github.com/u-root/u-root/cmds/core/init",
		"github.com/hugelgupf/vmtest/qemu/test/qemutest1",
	)
	if err := uroot.CreateInitramfs(logger, uopts); err != nil {
		t.Fatalf("error creating initramfs: %v", err)
	}

	vm, err := Start(
		GuestArch(),
		WithInitramfs(initrdPath),
		LogSerialByLine(PrintLineWithPrefix("vm", t.Logf)),
	)
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
}

func clearArgs() Fn {
	return func(alloc *IDAllocator, opts *Options) error {
		opts.QEMUArgs = nil
		opts.Kernel = ""
		opts.Initramfs = ""
		opts.KernelArgs = ""
		return nil
	}
}

func TestSubprocessTimesOut(t *testing.T) {
	vm, err := Start(ArchAMD64,
		WithQEMUCommand("sleep 30"),
		WithVMTimeout(5*time.Second),
		clearArgs(),
	)
	if err != nil {
		t.Fatalf("Failed to start 'VM': %v", err)
	}
	t.Logf("cmdline: %v", vm.CmdlineQuoted())

	var execErr *exec.ExitError
	err = vm.Wait()
	if !errors.As(err, &execErr) {
		t.Errorf("Failed to wait for VM: %v", err)
	}
	if execErr.Sys().(syscall.WaitStatus).Signal() != syscall.SIGKILL {
		t.Errorf("VM exited with %v, expected SIGKILL", err)
	}
}

func TestSubprocessKilled(t *testing.T) {
	vm, err := Start(ArchAMD64,
		WithQEMUCommand("sleep 60"),
		clearArgs(),
	)
	if err != nil {
		t.Fatalf("Failed to start 'VM': %v", err)
	}
	t.Logf("cmdline: %v", vm.CmdlineQuoted())

	if err := vm.Kill(); err != nil {
		t.Fatalf("Could not kill running subprocess: %v", err)
	}

	var execErr *exec.ExitError
	err = vm.Wait()
	if !errors.As(err, &execErr) {
		t.Errorf("Failed to wait for VM: %v", err)
	}
	if execErr.Sys().(syscall.WaitStatus).Signal() != syscall.SIGKILL {
		t.Errorf("VM exited with %v, expected SIGKILL", err)
	}
}

func TestTaskCanceledVMExits(t *testing.T) {
	var taskGotCanceled bool

	vm, err := Start(ArchAMD64,
		WithQEMUCommand("sleep 3"),
		clearArgs(),

		// Make sure that the test does not time out
		// forever -- context must get canceled.
		WithTask(func(ctx context.Context, n *Notifications) error {
			<-ctx.Done()
			taskGotCanceled = true
			return nil
		}),
	)
	if err != nil {
		t.Fatalf("Subprocess failed to start: %v", err)
	}
	t.Logf("cmdline: %v", vm.CmdlineQuoted())

	if err := vm.Wait(); err != nil {
		t.Fatalf("Subprocess exited with: %v", err)
	}

	if !taskGotCanceled {
		t.Error("Error: Task did not get canceled")
	}
}

func TestTaskCanceledIfVMFailsToStart(t *testing.T) {
	var taskGotCanceled bool

	_, err := Start(ArchAMD64,
		// Some path that does not exist.
		WithQEMUCommand(filepath.Join(t.TempDir(), "qemu")),
		// Make sure that the test does not time out
		// forever -- context must get canceled.
		WithTask(func(ctx context.Context, n *Notifications) error {
			<-ctx.Done()
			taskGotCanceled = true
			return nil
		}),
	)

	if !errors.Is(err, unix.ENOENT) {
		t.Fatalf("Failed to start VM: %v", err)
	}

	if !taskGotCanceled {
		t.Error("Error: Task did not get canceled")
	}
}

func TestExpectTimesOut(t *testing.T) {
	vm, err := Start(ArchAMD64,
		WithQEMUCommand("sleep 30"),
		WithVMTimeout(5*time.Second),
		clearArgs(),
	)
	if err != nil {
		t.Fatalf("Failed to start 'VM': %v", err)
	}
	t.Logf("cmdline: %v", vm.CmdlineQuoted())

	// TODO: Unfortunately the error we should expect doesn't indicate timeout.
	if _, err := vm.Console.ExpectString("literally anything"); err == nil {
		t.Errorf("Expect should have failed due to timeout")
	} else {
		t.Logf("error: %v", err)
	}

	var execErr *exec.ExitError
	if err := vm.Wait(); !errors.As(err, &execErr) {
		t.Errorf("Failed to wait for VM: %v", err)
	}
	if execErr.Sys().(syscall.WaitStatus).Signal() != syscall.SIGKILL {
		t.Errorf("VM exited with %v, expected SIGKILL", err)
	}
}

func TestWaitTwice(t *testing.T) {
	var errFoo = errors.New("foo")
	vm, err := Start(ArchAMD64,
		WithQEMUCommand("sleep 3"),
		clearArgs(),

		WithTask(Cleanup(func() error {
			return errFoo
		})),
	)
	if err != nil {
		t.Fatalf("Subprocess failed to start: %v", err)
	}
	t.Logf("cmdline: %v", vm.CmdlineQuoted())

	if err := vm.Wait(); !errors.Is(err, errFoo) {
		t.Fatalf("Wait = %v, want %v", err, errFoo)
	}

	if err := vm.Wait(); !errors.Is(err, errFoo) {
		t.Fatalf("Wait = %v, want %v", err, errFoo)
	}
}

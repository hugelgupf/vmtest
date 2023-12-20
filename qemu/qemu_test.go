// Copyright 2018 the u-root Authors. All rights reserved
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package qemu

import (
	"context"
	"errors"
	"fmt"
	"os"
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
		name string
		arch Arch
		fns  []Fn
		want []cmdlineEqualOpt
		envv map[string]string
		err  error
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
			name: "kernel-args-fail",
			arch: ArchAMD64,
			fns:  []Fn{WithQEMUCommand("qemu"), WithAppendKernel("printk=ttyS0")},
			err:  ErrKernelRequiredForArgs,
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
			opts, err := OptionsFor(tt.arch, tt.fns...)
			if err != nil {
				t.Errorf("Options = %v, want nil", err)
			}
			got, err := opts.Cmdline()
			if !errors.Is(err, tt.err) {
				t.Errorf("Cmdline = %v, want %v", err, tt.err)
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

func ClearQEMUArgs() Fn {
	return func(alloc *IDAllocator, opts *Options) error {
		opts.QEMUArgs = nil
		return nil
	}
}

func TestSubprocessTimesOut(t *testing.T) {
	vm, err := Start(ArchAMD64,
		WithQEMUCommand("sleep 30"),
		WithVMTimeout(5*time.Second),
		ClearQEMUArgs(),
		// In case the user is calling this test with env vars set.
		WithKernel(""),
		WithInitramfs(""),
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
		ClearQEMUArgs(),
		// In case the user is calling this test with env vars set.
		WithKernel(""),
		WithInitramfs(""),
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
		ClearQEMUArgs(),
		// In case the user is calling this test with env vars set.
		WithKernel(""),
		WithInitramfs(""),

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
		ClearQEMUArgs(),
		// In case the user is calling this test with env vars set.
		WithKernel(""),
		WithInitramfs(""),
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

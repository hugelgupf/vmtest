// Copyright 2018 the u-root Authors. All rights reserved
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package qemu

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"testing"

	"github.com/u-root/gobusybox/src/pkg/golang"
	"github.com/u-root/u-root/pkg/ulog/ulogtest"
	"github.com/u-root/u-root/pkg/uroot"
	"github.com/u-root/u-root/pkg/uroot/initramfs"
	"golang.org/x/exp/slices"
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
	for _, tt := range []struct {
		name string
		o    *Options
		want []cmdlineEqualOpt
		err  error
	}{
		{
			name: "simple",
			o: &Options{
				QEMUPath: "qemu",
				Kernel:   "./foobar",
			},
			want: []cmdlineEqualOpt{
				withArgv0("qemu"),
				withArg("-nographic"),
				withArg("-kernel", "./foobar"),
			},
		},
		{
			name: "kernel-args-fail",
			o: &Options{
				QEMUPath:   "qemu",
				KernelArgs: "printk=ttyS0",
			},
			err: ErrKernelRequiredForArgs,
		},
		{
			name: "device-kernel-args-fail",
			o: &Options{
				QEMUPath: "qemu",
				Devices:  []Device{ArbitraryKernelArgs{"earlyprintk=ttyS0"}},
			},
			err: ErrKernelRequiredForArgs,
		},
		{
			name: "kernel-args-initrd",
			o: &Options{
				QEMUPath:   "qemu",
				Kernel:     "./foobar",
				Initramfs:  "./initrd",
				KernelArgs: "printk=ttyS0",
				Devices:    []Device{ArbitraryKernelArgs{"earlyprintk=ttyS0"}},
			},
			want: []cmdlineEqualOpt{
				withArgv0("qemu"),
				withArg("-nographic"),
				withArg("-kernel", "./foobar"),
				withArg("-initrd", "./initrd"),
				withArg("-append", "printk=ttyS0 earlyprintk=ttyS0"),
			},
		},
		{
			name: "device-kernel-args",
			o: &Options{
				QEMUPath: "qemu",
				Kernel:   "./foobar",
				Devices:  []Device{ArbitraryKernelArgs{"earlyprintk=ttyS0"}},
			},
			want: []cmdlineEqualOpt{
				withArgv0("qemu"),
				withArg("-nographic"),
				withArg("-kernel", "./foobar"),
				withArg("-append", "earlyprintk=ttyS0"),
			},
		},
		{
			name: "id-allocator",
			o: &Options{
				QEMUPath: "qemu",
				Kernel:   "./foobar",
				Devices: []Device{
					IDEBlockDevice{"./disk1"},
					IDEBlockDevice{"./disk2"},
				},
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
	} {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tt.o.Cmdline()
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

func guestGOARCH() string {
	if env := os.Getenv("VMTEST_GOARCH"); env != "" {
		return env
	}
	return runtime.GOARCH
}

func TestStartVM(t *testing.T) {
	tmp := t.TempDir()
	logger := &ulogtest.Logger{TB: t}
	initrdPath := filepath.Join(tmp, "initramfs.cpio")
	initrdWriter, err := initramfs.CPIO.OpenWriter(logger, initrdPath)
	if err != nil {
		t.Fatalf("Failed to create initramfs writer: %v", err)
	}

	env := golang.Default()
	env.CgoEnabled = false
	env.GOARCH = guestGOARCH()

	uopts := uroot.Opts{
		Env:        &env,
		InitCmd:    "init",
		UinitCmd:   "qemutest1",
		OutputFile: initrdWriter,
		TempDir:    tmp,
	}
	uopts.AddBusyBoxCommands(
		"github.com/u-root/u-root/cmds/core/init",
		"github.com/hugelgupf/vmtest/qemu/qemutest1",
	)
	if err := uroot.CreateInitramfs(logger, uopts); err != nil {
		t.Fatalf("error creating initramfs: %v", err)
	}

	r, w := io.Pipe()
	opts := &Options{
		QEMUArch:     GOARCHToQEMUArch[guestGOARCH()],
		Kernel:       os.Getenv("VMTEST_KERNEL"),
		Initramfs:    initrdPath,
		SerialOutput: w,
	}
	if arch, err := opts.Arch(); err != nil {
		t.Fatal(err)
	} else if arch == "arm" {
		opts.KernelArgs = "console=ttyAMA0"
	} else if arch == "x86_64" {
		opts.KernelArgs = "console=ttyS0 earlyprintk=ttyS0"
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

	vm, err := opts.Start()
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

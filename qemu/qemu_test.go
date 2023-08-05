// Copyright 2018 the u-root Authors. All rights reserved
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package qemu

import (
	"errors"
	"fmt"
	"testing"

	"golang.org/x/exp/slices"
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
			name: "kernel-args",
			o: &Options{
				QEMUPath:   "qemu",
				Kernel:     "./foobar",
				KernelArgs: "printk=ttyS0",
				Devices:    []Device{ArbitraryKernelArgs{"earlyprintk=ttyS0"}},
			},
			want: []cmdlineEqualOpt{
				withArgv0("qemu"),
				withArg("-nographic"),
				withArg("-kernel", "./foobar"),
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

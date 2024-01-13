// Copyright 2018 the u-root Authors. All rights reserved
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package qemu

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"syscall"
	"testing"
)

func TestIDAllocator(t *testing.T) {
	tc := []struct {
		in   string
		want string
	}{
		{in: "pipe", want: "pipe0"},
		{in: "pipe", want: "pipe1"},
		{in: "pipe0", want: "pipe2"},
		{in: "pipe45", want: "pipe3"},
		{in: "0pipe34", want: "0pipe0"},
		{in: "pip", want: "pip0"},
		{in: "id", want: "id0"},
		{in: "pip", want: "pip1"},
	}
	a := NewIDAllocator()
	for _, c := range tc {
		got := a.ID(c.in)
		if got != c.want {
			t.Errorf("ID(%s) = %s, want %s", c.in, got, c.want)
		}
	}
}

func TestDevices(t *testing.T) {
	emptyFilePath := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(emptyFilePath, []byte{}, 0o777); err != nil {
		t.Fatal(err)
	}

	dir := t.TempDir()
	for _, tt := range []struct {
		name string
		arch Arch
		fns  []Fn
		want []cmdlineEqualOpt
		err  error
	}{
		{
			name: "ide-read-only-dir",
			arch: ArchAMD64,
			fns:  []Fn{WithQEMUCommand("qemu"), WithKernel("./foobar"), ReadOnlyDirectory(dir)},
			want: []cmdlineEqualOpt{
				withArgv0("qemu"),
				withArg("-nographic"),
				withArg("-kernel", "./foobar"),
				withArg("-drive", fmt.Sprintf("file=fat:rw:%s,if=none,id=drive0", dir),
					"-device", "ich9-ahci,id=ahci0",
					"-device", "ide-hd,drive=drive0,bus=ahci0.0"),
			},
		},
		{
			name: "9p-missing-dir",
			arch: ArchAMD64,
			fns:  []Fn{P9Directory("", "tag")},
			err:  ErrInvalidDir,
		},
		{
			name: "9p-missing-tag",
			arch: ArchAMD64,
			fns:  []Fn{P9Directory(t.TempDir(), "")},
			err:  ErrInvalidTag,
		},
		{
			name: "9p-not-a-dir",
			arch: ArchAMD64,
			fns:  []Fn{P9Directory(emptyFilePath, "tag")},
			err:  ErrIsNotDir,
		},
		{
			name: "9p-not-exist",
			arch: ArchAMD64,
			fns:  []Fn{P9Directory(filepath.Join(t.TempDir(), "non-exist"), "tag")},
			err:  syscall.ENOENT,
		},
		{
			name: "ide-missing-dir",
			arch: ArchAMD64,
			fns:  []Fn{ReadOnlyDirectory("")},
			err:  ErrInvalidDir,
		},
		{
			name: "ide-not-a-dir",
			arch: ArchAMD64,
			fns:  []Fn{ReadOnlyDirectory(emptyFilePath)},
			err:  ErrIsNotDir,
		},
		{
			name: "ide-not-exist",
			arch: ArchAMD64,
			fns:  []Fn{ReadOnlyDirectory(filepath.Join(t.TempDir(), "non-exist"))},
			err:  syscall.ENOENT,
		},
		{
			name: "ide-block-not-exist",
			arch: ArchAMD64,
			fns:  []Fn{IDEBlockDevice(filepath.Join(t.TempDir(), "non-exist"))},
			err:  syscall.ENOENT,
		},
		{
			name: "by-arch-found",
			arch: ArchAMD64,
			fns: []Fn{
				WithQEMUCommand("qemu"),
				ByArch(map[Arch]Fn{
					ArchAMD64: ArbitraryArgs("-game"),
					ArchArm:   ArbitraryArgs("-foobar"),
				}),
			},
			want: []cmdlineEqualOpt{
				withArgv0("qemu"),
				withArg("-nographic"),
				withArg("-game"),
			},
		},
		{
			name: "by-arch-not-found",
			arch: ArchAMD64,
			fns: []Fn{
				WithQEMUCommand("qemu"),
				ByArch(map[Arch]Fn{
					ArchArm64: ArbitraryArgs("-game"),
					ArchArm:   ArbitraryArgs("-foobar"),
				}),
			},
			want: []cmdlineEqualOpt{
				withArgv0("qemu"),
				withArg("-nographic"),
			},
		},
		{
			name: "all-ifs",
			arch: ArchAMD64,
			fns: []Fn{
				WithQEMUCommand("qemu"),
				All(
					IfArch(ArchAMD64, ArbitraryArgs("-game")),
					IfArch(ArchArm, ArbitraryArgs("-notgame")),
					IfNotArch(ArchAMD64, ArbitraryArgs("-notfoobar")),
					IfNotArch(ArchArm, ArbitraryArgs("-foobar")),
				),
			},
			want: []cmdlineEqualOpt{
				withArgv0("qemu"),
				withArg("-nographic"),
				withArg("-game"),
				withArg("-foobar"),
			},
		},
		{
			name: "all-error",
			arch: ArchAMD64,
			fns: []Fn{
				WithQEMUCommand("qemu"),
				All(
					IfArch(ArchAMD64, ArbitraryArgs("-game")),
					P9Directory("", "tag"),
					func(alloc *IDAllocator, opts *Options) error {
						panic("not run!")
					},
				),
			},
			err: ErrInvalidDir,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			opts, err := OptionsFor(tt.arch, tt.fns...)
			if !errors.Is(err, tt.err) {
				t.Errorf("Options = %v, want %v", err, tt.err)
			}
			if opts == nil {
				return
			}
			got, err := opts.Cmdline()
			if err != nil {
				t.Errorf("Cmdline = %v, want nil", err)
			}

			t.Logf("Got cmdline: %v", got)
			if err := isCmdlineEqual(got, tt.want...); err != nil {
				t.Errorf("Cmdline = %v", err)
			}
		})
	}
}

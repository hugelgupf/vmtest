// Copyright 2021 the u-root Authors. All rights reserved
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package common has commonly used functions in guest VM test runners.
package common

import (
	"fmt"
	"os"

	"github.com/u-root/u-root/pkg/mount"
	"golang.org/x/sys/unix"
)

const (
	envUse9P  = "UROOT_USE_9P"
	sharedDir = "/testdata"

	// https://wiki.qemu.org/Documentation/9psetup#msize recommends an
	// msize of at least 10MiB. Larger number might give better
	// performance. QEMU will print a warning if it is too small. Linux's
	// default is 8KiB which is way too small.
	msize9P = 10 * 1024 * 1024
)

// MountSharedDir mounts the directory shared with the VM test. A cleanup
// function is returned to unmount.
func MountSharedDir() (func(), error) {
	// Mount a disk and run the tests within.
	var (
		mp  *mount.MountPoint
		err error
	)

	if err := os.MkdirAll(sharedDir, 0o644); err != nil {
		return nil, err
	}

	if os.Getenv(envUse9P) == "1" {
		mp, err = mount.Mount("tmpdir", sharedDir, "9p", fmt.Sprintf("9P2000.L,msize=%d", msize9P), 0)
	} else {
		mp, err = mount.Mount("/dev/sda1", sharedDir, "vfat", "", unix.MS_RDONLY)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to mount test directory: %v", err)
	}
	return func() { _ = mp.Unmount(0) }, nil
}

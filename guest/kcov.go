// Copyright 2021 the u-root Authors. All rights reserved
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package guest

import (
	"archive/tar"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/u-root/u-root/pkg/mount"
	"github.com/u-root/u-root/pkg/tarutil"
)

const (
	// https://wiki.qemu.org/Documentation/9psetup#msize recommends an
	// msize of at least 10MiB. Larger number might give better
	// performance. QEMU will print a warning if it is too small. Linux's
	// default is 8KiB which is way too small.
	msize9P = 10 * 1024 * 1024
)

// gcovFilter filters on all files ending with a gcda or gcno extension.
func gcovFilter(hdr *tar.Header) bool {
	if hdr.Typeflag == tar.TypeDir {
		hdr.Mode = 0o770
		return true
	}
	if (filepath.Ext(hdr.Name) == ".gcda" && hdr.Typeflag == tar.TypeReg) ||
		(filepath.Ext(hdr.Name) == ".gcno" && hdr.Typeflag == tar.TypeSymlink) {
		hdr.Mode = 0o660
		return true
	}
	return false
}

// CollectKernelCoverage saves the kernel coverage report to a tar file.
func CollectKernelCoverage() {
	tag := os.Getenv("VMTEST_KCOVERAGE_TAG")
	if tag == "" {
		log.Printf("Kernel coverage collection skipped.")
		return
	}

	coverageDir := "/coverage"
	if err := os.MkdirAll(coverageDir, 0o644); err != nil {
		log.Fatal(err)
	}

	mp, err := mount.Mount(tag, coverageDir, "9p", fmt.Sprintf("9P2000.L,msize=%d", msize9P), 0)
	if err != nil {
		log.Fatalf("Failed to mount coverage directory: %v", err)
	}
	defer func() { _ = mp.Unmount(0) }()

	if err := collectKernelCoverage(filepath.Join(coverageDir, "kernel_coverage.tar")); err != nil {
		log.Printf("Failed to collect kernel coverage: %v", err)
	}
}

func collectKernelCoverage(filename string) error {
	gcovDir := "/sys/kernel/debug/gcov"
	if _, err := os.Stat(gcovDir); os.IsNotExist(err) {
		return fmt.Errorf("kernel coverage cannot be collected because %q does not exist (is the kernel compiled with CONFIG_GCOV_KERNEL?)", gcovDir)
	}

	// Copy out the kernel code coverage.
	log.Print("Collecting kernel coverage...")
	f, err := os.Create(filename)
	if err != nil {
		return err
	}
	if err := tarutil.CreateTar(f, []string{strings.TrimLeft(gcovDir, "/")}, &tarutil.Opts{
		Filters: []tarutil.Filter{gcovFilter},
		// Make sure the files are not stored absolute; otherwise, they
		// become difficult to extract safely.
		ChangeDirectory: "/",
	}); err != nil {
		f.Close()
		return err
	}
	// Sync to "disk" because we are about to shut down the kernel.
	if err := f.Sync(); err != nil {
		f.Close()
		return fmt.Errorf("error syncing: %v", err)
	}
	if err := f.Close(); err != nil {
		return fmt.Errorf("error closing: %v", err)
	}
	return nil
}

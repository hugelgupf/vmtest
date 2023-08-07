// Copyright 2018 the u-root Authors. All rights reserved
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package qemu

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net"
	"strings"
	"sync/atomic"
)

// IDAllocator is used to ensure no overlapping QEMU option IDs.
type IDAllocator struct {
	// maps a prefix to the maximum used suffix number.
	idx map[string]uint32
}

// NewIDAllocator returns a new ID allocator for QEMU option IDs.
func NewIDAllocator() *IDAllocator {
	return &IDAllocator{
		idx: make(map[string]uint32),
	}
}

// ID returns the next available ID for the given prefix.
func (a *IDAllocator) ID(prefix string) string {
	prefix = strings.TrimRight(prefix, "0123456789")
	idx := a.idx[prefix]
	a.idx[prefix]++
	return fmt.Sprintf("%s%d", prefix, idx)
}

// Network is a Device that can connect multiple QEMU VMs to each other.
//
// Network uses the QEMU socket mechanism to connect multiple VMs with a simple
// TCP socket.
type Network struct {
	port uint16

	// numVMs must be atomically accessed so VMs can be started in parallel
	// in goroutines.
	numVMs uint32
}

// NewNetwork creates a new QEMU network between QEMU VMs.
//
// The network is closed from the world and only between the QEMU VMs.
func NewNetwork() *Network {
	return &Network{
		port: 1234,
	}
}

// NetworkOpt returns additional QEMU command-line parameters based on the net
// device ID.
type NetworkOpt func(netdev string, id *IDAllocator) []string

// WithPCAP captures network traffic and saves it to outputFile.
func WithPCAP(outputFile string) NetworkOpt {
	return func(netdev string, id *IDAllocator) []string {
		return []string{
			"-object",
			fmt.Sprintf("filter-dump,id=%s,netdev=%s,file=%s", id.ID("filter"), netdev, outputFile),
		}
	}
}

// NewVM returns a Device that can be used with a new QEMU VM.
func (n *Network) NewVM(nopts ...NetworkOpt) Fn {
	if n == nil {
		return nil
	}

	newNum := atomic.AddUint32(&n.numVMs, 1)
	num := newNum - 1

	// MAC for the virtualized NIC.
	//
	// This is from the range of locally administered address ranges.
	mac := net.HardwareAddr{0x0e, 0x00, 0x00, 0x00, 0x00, byte(num)}
	return func(alloc *IDAllocator, opts *Options) error {
		devID := alloc.ID("vm")

		args := []string{"-device", fmt.Sprintf("e1000,netdev=%s,mac=%s", devID, mac)}
		// Note: QEMU in CircleCI seems to in solve cases fail when using just ':1234' format.
		// It fails with "address resolution failed for :1234: Name or service not known"
		// hinting that this is somehow related to DNS resolution. To work around this,
		// we explicitly bind to 127.0.0.1 (IPv6 [::1] is not parsed correctly by QEMU).
		if num != 0 {
			args = append(args, "-netdev", fmt.Sprintf("socket,id=%s,connect=127.0.0.1:%d", devID, n.port))
		} else {
			args = append(args, "-netdev", fmt.Sprintf("socket,id=%s,listen=127.0.0.1:%d", devID, n.port))
		}

		for _, opt := range nopts {
			args = append(args, opt(devID, alloc)...)
		}
		opts.AppendQEMU(args...)
		return nil
	}
}

// ReadOnlyDirectory is a Device that exposes a directory as a /dev/sda1
// readonly vfat partition in the VM.
func ReadOnlyDirectory(dir string) Fn {
	return func(alloc *IDAllocator, opts *Options) error {
		if len(dir) == 0 {
			return nil
		}

		drive := alloc.ID("drive")
		ahci := alloc.ID("ahci")

		// Expose the temp directory to QEMU as /dev/sda1
		opts.AppendQEMU(
			"-drive", fmt.Sprintf("file=fat:rw:%s,if=none,id=%s", dir, drive),
			"-device", fmt.Sprintf("ich9-ahci,id=%s", ahci),
			"-device", fmt.Sprintf("ide-hd,drive=%s,bus=%s.0", drive, ahci),
		)
		return nil
	}
}

// IDEBlockDevice emulates an AHCI/IDE block device.
func IDEBlockDevice(file string) Fn {
	return func(alloc *IDAllocator, opts *Options) error {
		if len(file) == 0 {
			return nil
		}

		drive := alloc.ID("drive")
		ahci := alloc.ID("ahci")

		opts.AppendQEMU(
			"-drive", fmt.Sprintf("file=%s,if=none,id=%s", file, drive),
			"-device", fmt.Sprintf("ich9-ahci,id=%s", ahci),
			"-device", fmt.Sprintf("ide-hd,drive=%s,bus=%s.0", drive, ahci),
		)
		return nil
	}
}

// P9Directory is a Device that exposes a directory as a Plan9 (9p)
// read-write filesystem in the VM.
// dir is the directory to expose as read-write 9p filesystem.
//
// If boot is true, indicates this is the root volume. There
// can only be one boot 9pfs at a time.
//
// tag is an identifier that is used within the VM when mounting an fs,
// e.g. 'mount -t 9p my-vol-ident mountpoint'. If not specified, a default
// tag of 'tmpdir' will be used, unless boot is set, in which case
// "rootdrv" is set (a special value Linux knows how to interpret).
//
// Because the tag must be unique for each dir, if multiple non-boot
// P9Directory's are used, tag may be omitted for no more than one.
func P9Directory(dir string, boot bool, tag string) Fn {
	return func(alloc *IDAllocator, opts *Options) error {
		if len(dir) == 0 {
			return nil
		}

		var id string
		if boot {
			tag = "/dev/root"
		} else if len(tag) == 0 {
			tag = "tmpdir"
		}
		if boot {
			id = "rootdrv"
		} else {
			id = alloc.ID("fsdev")
		}

		// Expose the temp directory to QEMU
		var deviceArgs string
		switch opts.GuestArch() {
		case GuestArchArm:
			deviceArgs = fmt.Sprintf("virtio-9p-device,fsdev=%s,mount_tag=%s", id, tag)
		default:
			deviceArgs = fmt.Sprintf("virtio-9p-pci,fsdev=%s,mount_tag=%s", id, tag)
		}

		opts.AppendQEMU(
			// security_model=mapped-file seems to be the best choice. It gives
			// us control over uid/gid/mode seen in the guest, without requiring
			// elevated perms on the host.
			"-fsdev", fmt.Sprintf("local,id=%s,path=%s,security_model=mapped-file", id, dir),
			"-device", deviceArgs,
		)
		if boot {
			opts.AppendKernel(
				"devtmpfs.mount=1",
				"root=/dev/root",
				"rootfstype=9p",
				"rootflags=trans=virtio,version=9p2000.L",
			)
		} else {
			// seen as an env var by the init process
			opts.AppendKernel("UROOT_USE_9P=1")
		}
		return nil
	}
}

// VirtioRandom is a Device that exposes a PCI random number generator to the
// QEMU VM.
func VirtioRandom() Fn {
	return ArbitraryArgs("-device", "virtio-rng-pci")
}

// ArbitraryArgs is a Device that allows users to add arbitrary arguments to
// the QEMU command line.
func ArbitraryArgs(aa ...string) Fn {
	return func(alloc *IDAllocator, opts *Options) error {
		opts.AppendQEMU(aa...)
		return nil
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

// LogSerialByLine processes serial output from the guest one line at a time
// and calls callback on each full line.
func LogSerialByLine(callback func(line string)) Fn {
	return func(alloc *IDAllocator, opts *Options) error {
		r, w := io.Pipe()
		opts.SerialOutput = append(opts.SerialOutput, w)
		opts.Tasks = append(opts.Tasks, WaitVMStarted(func(ctx context.Context, n *Notifications) error {
			s := bufio.NewScanner(r)
			for s.Scan() {
				callback(string(replaceCtl(s.Bytes())))
			}
			if err := s.Err(); err != nil {
				return fmt.Errorf("error reading serial from VM: %w", err)
			}
			return nil
		}))
		return nil
	}
}

// PrintLineWithPrefix returns a usable callback for LogSerialByLine that
// prints a prefix and the line. Usable with any standard Go print function
// like t.Logf or fmt.Printf.
func PrintLineWithPrefix(prefix string, printer func(fmt string, arg ...any)) func(line string) {
	return func(line string) {
		printer("%s: %s", prefix, line)
	}
}

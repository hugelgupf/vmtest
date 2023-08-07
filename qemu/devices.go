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

// Device is a QEMU device to expose to a VM.
type Device interface {
	// Cmdline returns arguments to append to the QEMU command line for this device.
	Cmdline(arch GuestArch, id *IDAllocator) []string

	// KArgs returns arguments that must be passed to the kernel for this device, or nil.
	KArgs() []string
}

// WithDevice adds a device to the VM.
func WithDevice(d Device) Fn {
	return func(alloc *IDAllocator, opts *Options) error {
		opts.AppendQEMU(d.Cmdline(opts.GuestArch(), alloc)...)
		opts.AppendKernel(d.KArgs()...)
		return nil
	}
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
func (n *Network) NewVM(nopts ...NetworkOpt) Device {
	if n == nil {
		return nil
	}

	newNum := atomic.AddUint32(&n.numVMs, 1)
	num := newNum - 1

	// MAC for the virtualized NIC.
	//
	// This is from the range of locally administered address ranges.
	mac := net.HardwareAddr{0x0e, 0x00, 0x00, 0x00, 0x00, byte(num)}
	return networkImpl{
		port:    n.port,
		nopts:   nopts,
		mac:     mac,
		connect: num != 0,
	}
}

type networkImpl struct {
	port    uint16
	nopts   []NetworkOpt
	mac     net.HardwareAddr
	connect bool
}

func (n networkImpl) Cmdline(arch GuestArch, id *IDAllocator) []string {
	devID := id.ID("vm")

	args := []string{"-device", fmt.Sprintf("e1000,netdev=%s,mac=%s", devID, n.mac)}
	// Note: QEMU in CircleCI seems to in solve cases fail when using just ':1234' format.
	// It fails with "address resolution failed for :1234: Name or service not known"
	// hinting that this is somehow related to DNS resolution. To work around this,
	// we explicitly bind to 127.0.0.1 (IPv6 [::1] is not parsed correctly by QEMU).
	if n.connect {
		args = append(args, "-netdev", fmt.Sprintf("socket,id=%s,connect=127.0.0.1:%d", devID, n.port))
	} else {
		args = append(args, "-netdev", fmt.Sprintf("socket,id=%s,listen=127.0.0.1:%d", devID, n.port))
	}

	for _, opt := range n.nopts {
		args = append(args, opt(devID, id)...)
	}
	return args
}

func (n networkImpl) KArgs() []string { return nil }

// ReadOnlyDirectory is a Device that exposes a directory as a /dev/sda1
// readonly vfat partition in the VM.
type ReadOnlyDirectory struct {
	// Dir is the directory to expose as a read-only vfat partition.
	Dir string
}

func (rod ReadOnlyDirectory) Cmdline(arch GuestArch, id *IDAllocator) []string {
	if len(rod.Dir) == 0 {
		return nil
	}

	drive := id.ID("drive")
	ahci := id.ID("ahci")

	// Expose the temp directory to QEMU as /dev/sda1
	return []string{
		"-drive", fmt.Sprintf("file=fat:rw:%s,if=none,id=%s", rod.Dir, drive),
		"-device", fmt.Sprintf("ich9-ahci,id=%s", ahci),
		"-device", fmt.Sprintf("ide-hd,drive=%s,bus=%s.0", drive, ahci),
	}
}

func (ReadOnlyDirectory) KArgs() []string { return nil }

// IDEBlockDevice emulates an AHCI/IDE block device.
type IDEBlockDevice struct {
	File string
}

func (ibd IDEBlockDevice) Cmdline(arch GuestArch, id *IDAllocator) []string {
	if len(ibd.File) == 0 {
		return nil
	}

	drive := id.ID("drive")
	ahci := id.ID("ahci")

	return []string{
		"-drive", fmt.Sprintf("file=%s,if=none,id=%s", ibd.File, drive),
		"-device", fmt.Sprintf("ich9-ahci,id=%s", ahci),
		"-device", fmt.Sprintf("ide-hd,drive=%s,bus=%s.0", drive, ahci),
	}
}

func (IDEBlockDevice) KArgs() []string { return nil }

// P9Directory is a Device that exposes a directory as a Plan9 (9p)
// read-write filesystem in the VM.
type P9Directory struct {
	// Dir is the directory to expose as read-write 9p filesystem.
	Dir string

	// Boot: if true, indicates this is the root volume. For this to work,
	// kernel args will need to be added - use KArgs() to get the args. There
	// can only be one boot 9pfs at a time.
	Boot bool

	// Tag is an identifier that is used within the VM when mounting an fs,
	// e.g. 'mount -t 9p my-vol-ident mountpoint'. If not specified, a default
	// tag of 'tmpdir' will be used.
	//
	// Ignored if Boot is true, as the tag in that case is special.
	//
	// For non-boot devices, this is also used as the id linking the `-fsdev`
	// and `-device` args together.
	//
	// Because the tag must be unique for each dir, if multiple non-boot
	// P9Directory's are used, tag may be omitted for no more than one.
	Tag string
}

func (p P9Directory) Cmdline(arch GuestArch, ida *IDAllocator) []string {
	if len(p.Dir) == 0 {
		return nil
	}

	var tag, id string
	if p.Boot {
		tag = "/dev/root"
	} else {
		tag = p.Tag
		if len(tag) == 0 {
			tag = "tmpdir"
		}
	}
	if p.Boot {
		id = "rootdrv"
	} else {
		id = tag
	}

	// Expose the temp directory to QEMU
	var deviceArgs string
	switch arch {
	case GuestArchArm:
		deviceArgs = fmt.Sprintf("virtio-9p-device,fsdev=%s,mount_tag=%s", id, tag)
	default:
		deviceArgs = fmt.Sprintf("virtio-9p-pci,fsdev=%s,mount_tag=%s", id, tag)
	}

	return []string{
		// security_model=mapped-file seems to be the best choice. It gives
		// us control over uid/gid/mode seen in the guest, without requiring
		// elevated perms on the host.
		"-fsdev", fmt.Sprintf("local,id=%s,path=%s,security_model=mapped-file", id, p.Dir),
		"-device", deviceArgs,
	}
}

func (p P9Directory) KArgs() []string {
	if len(p.Dir) == 0 {
		return nil
	}
	if p.Boot {
		return []string{
			"devtmpfs.mount=1",
			"root=/dev/root",
			"rootfstype=9p",
			"rootflags=trans=virtio,version=9p2000.L",
		}
	}
	return []string{
		// seen as an env var by the init process
		"UROOT_USE_9P=1",
	}
}

// VirtioRandom is a Device that exposes a PCI random number generator to the
// QEMU VM.
func VirtioRandom(alloc *IDAllocator, opts *Options) error {
	opts.AppendQEMU("-device", "virtio-rng-pci")
	return nil
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
		opts.SerialOutput = w
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

// Copyright 2018 the u-root Authors. All rights reserved
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package qemu

import (
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/hugelgupf/vmtest/internal/eventchannel"
)

type VMStarter struct {
	ID             *IDAllocator
	argv           []string
	kernelArgs     []string
	extraFiles     []*os.File
	preExitWaitFn  []func() error
	postExitWaitFn []func() error
}

func (vms *VMStarter) AppendQEMU(arg ...string) {
	vms.argv = append(vms.argv, arg...)
}

func (vms *VMStarter) AppendKernel(arg ...string) {
	vms.kernelArgs = append(vms.kernelArgs, arg...)
}

// AddFile adds the file to the QEMU process and returns the FD it will be in
// the child process.
func (vms *VMStarter) AddFile(f *os.File) int {
	vms.extraFiles = append(vms.extraFiles, f)

	// 0, 1, 2 used for stdin/out/err.
	return len(vms.extraFiles) + 2
}

// PreExitWaitGoroutine adds a goroutine that is started after the QEMU
// process is started, and will be waited on as part of QEMU process exit.
func (vms *VMStarter) PreExitWaitGoroutine(fn func() error) {
	vms.preExitWaitFn = append(vms.preExitWaitFn, fn)
}

// PostExitWaitGoroutine adds a goroutine that is started after the QEMU
// process is started, and will be waited on as part of QEMU process exit.
func (vms *VMStarter) PostExitWaitGoroutine(fn func() error) {
	vms.postExitWaitFn = append(vms.postExitWaitFn, fn)
}

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
	// Setup appends arguments to the QEMU + kernel command line for this device.
	Setup(arch string, setup *VMStarter)
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
type NetworkOpt func(netdev string, setup *VMStarter) []string

// WithPCAP captures network traffic and saves it to outputFile.
func WithPCAP(outputFile string) NetworkOpt {
	return func(netdev string, setup *VMStarter) []string {
		return []string{
			"-object",
			fmt.Sprintf("filter-dump,id=%s,netdev=%s,file=%s", setup.ID.ID("filter"), netdev, outputFile),
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

func (n networkImpl) Setup(arch string, vms *VMStarter) {
	devID := vms.ID.ID("vm")

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
		args = append(args, opt(devID, vms)...)
	}
	vms.AppendQEMU(args...)
}

// ReadOnlyDirectory is a Device that exposes a directory as a /dev/sda1
// readonly vfat partition in the VM.
type ReadOnlyDirectory struct {
	// Dir is the directory to expose as a read-only vfat partition.
	Dir string
}

func (rod ReadOnlyDirectory) Setup(arch string, vms *VMStarter) {
	if len(rod.Dir) == 0 {
		return
	}

	drive := vms.ID.ID("drive")
	ahci := vms.ID.ID("ahci")

	// Expose the temp directory to QEMU as /dev/sda1
	vms.AppendQEMU(
		"-drive", fmt.Sprintf("file=fat:rw:%s,if=none,id=%s", rod.Dir, drive),
		"-device", fmt.Sprintf("ich9-ahci,id=%s", ahci),
		"-device", fmt.Sprintf("ide-hd,drive=%s,bus=%s.0", drive, ahci),
	)
}

// IDEBlockDevice emulates an AHCI/IDE block device.
type IDEBlockDevice struct {
	File string
}

func (ibd IDEBlockDevice) Setup(arch string, vms *VMStarter) {
	if len(ibd.File) == 0 {
		return
	}

	drive := vms.ID.ID("drive")
	ahci := vms.ID.ID("ahci")

	vms.AppendQEMU(
		"-drive", fmt.Sprintf("file=%s,if=none,id=%s", ibd.File, drive),
		"-device", fmt.Sprintf("ich9-ahci,id=%s", ahci),
		"-device", fmt.Sprintf("ide-hd,drive=%s,bus=%s.0", drive, ahci),
	)
}

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

func (p P9Directory) Setup(arch string, vms *VMStarter) {
	if len(p.Dir) == 0 {
		return
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
	case "arm":
		deviceArgs = fmt.Sprintf("virtio-9p-device,fsdev=%s,mount_tag=%s", id, tag)
	default:
		deviceArgs = fmt.Sprintf("virtio-9p-pci,fsdev=%s,mount_tag=%s", id, tag)
	}

	vms.AppendQEMU(
		// security_model=mapped-file seems to be the best choice. It gives
		// us control over uid/gid/mode seen in the guest, without requiring
		// elevated perms on the host.
		"-fsdev", fmt.Sprintf("local,id=%s,path=%s,security_model=mapped-file", id, p.Dir),
		"-device", deviceArgs,
	)

	if p.Boot {
		vms.AppendKernel(
			"devtmpfs.mount=1",
			"root=/dev/root",
			"rootfstype=9p",
			"rootflags=trans=virtio,version=9p2000.L",
		)
	} else {
		// seen as an env var by the init process
		vms.AppendKernel("UROOT_USE_9P=1")
	}
}

// VirtioRandom is a Device that exposes a PCI random number generator to the
// QEMU VM.
type VirtioRandom struct{}

func (VirtioRandom) Setup(arch string, vms *VMStarter) {
	vms.AppendQEMU("-device", "virtio-rng-pci")
}

// ArbitraryArgs is a Device that allows users to add arbitrary arguments to
// the QEMU command line.
type ArbitraryArgs []string

func (aa ArbitraryArgs) Setup(arch string, vms *VMStarter) {
	vms.AppendQEMU(aa...)
}

// VirtioSerial exposes a named pipe virtio-serial device to the guest.
//
// The guest can find the device by finding /sys/class/virtio-ports/$dev/name
// that contains the text of the Name member. /dev/$dev will be the
// communication serial device.
type VirtioSerial struct {
	// NamedPipePath is the name of the pipe to write to.
	NamedPipePath string

	// Name of the device. Guest can use this name to discover the device.
	Name string
}

func (s VirtioSerial) Setup(arch string, vms *VMStarter) {
	pipeID := vms.ID.ID("pipe")
	vms.AppendQEMU(
		"-device", "virtio-serial",
		"-chardev", fmt.Sprintf("pipe,id=%s,path=%s", pipeID, s.NamedPipePath),
		"-device", fmt.Sprintf("virtserialport,chardev=%s,name=%s", pipeID, s.Name),
	)
}

// NewEventChannel ...
func NewEventChannel[T any](name string, callback func(T)) (Device, error) {
	r, w, err := os.Pipe()
	if err != nil {
		return nil, err
	}

	return &eventChannel[T]{
		r:       r,
		w:       w,
		handler: callback,
		name:    name,
	}, nil
}

type eventChannel[T any] struct {
	r, w    *os.File
	handler func(T)
	name    string
}

func (e eventChannel[T]) Setup(arch string, vms *VMStarter) {
	fd := vms.AddFile(e.w)

	pipeID := vms.ID.ID("pipe")
	vms.AppendQEMU(
		"-device", "virtio-serial",
		"-chardev", fmt.Sprintf("pipe,id=%s,path=/proc/self/fd/%d", pipeID, fd),
		"-device", fmt.Sprintf("virtserialport,chardev=%s,name=%s", pipeID, e.name),
	)

	r, w := io.Pipe()

	var wg sync.WaitGroup
	wg.Add(1)
	vms.PostExitWaitGoroutine(func() error {
		// Close write-end on parent side.
		e.w.Close()

		go func() {
			for {
				b := make([]byte, 1024)
				n, err := e.r.Read(b[:])
				if err == io.EOF || (err == nil && n == 0) {
					log.Printf("close")
					w.Close()
					return
				}
				m, err := w.Write(b[:n])
				if err != nil {
					log.Printf("failed to write: %v", err)
				} else if m != n {
					log.Printf("m != n")
				}
				log.Printf("received: %s", string(b[:n]))
			}
		}()

		return eventchannel.ProcessJSONByLine[eventchannel.Event[T]](r, func(c eventchannel.Event[T]) {
			switch c.GuestAction {
			case eventchannel.ActionGuestEvent:
				e.handler(c.Actual)

			case eventchannel.ActionDone:
				log.Printf("got done")
				wg.Done()
			}
		})
	})
	vms.PreExitWaitGoroutine(func() error {
		log.Printf("waiting for done")
		wg.Wait()
		return nil
	})
}

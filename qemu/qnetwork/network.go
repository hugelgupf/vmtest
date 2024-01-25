// Copyright 2018 the u-root Authors. All rights reserved
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package qnetwork provides net device configurators for use with the Go qemu
// API.
package qnetwork

import (
	"fmt"
	"net"
	"os"
	"path/filepath"
	"sync/atomic"

	"github.com/hugelgupf/vmtest/qemu"
)

// NIC is a QEMU NIC device string.
//
// Valid values for your QEMU can be found with	`qemu-system-<arch> -device
// help` in the Network devices section.
type NIC string

// A subset of QEMU NIC devices.
const (
	NICE1000     NIC = "e1000"
	NICVirtioNet NIC = "virtio-net"
)

// Options are network device options.
type Options struct {
	// NIC is the NIC device that QEMU emulates.
	NIC NIC

	// MAC is the MAC address assigned to this interface in the guest.
	MAC net.HardwareAddr
}

// Opt returns additional QEMU command-line parameters based on the net
// device ID.
type Opt func(netdev string, id *qemu.IDAllocator, opts *Options) []string

// WithPCAP captures network traffic and saves it to outputFile.
func WithPCAP(outputFile string) Opt {
	return func(netdev string, id *qemu.IDAllocator, opts *Options) []string {
		return []string{
			"-object",
			fmt.Sprintf("filter-dump,id=%s,netdev=%s,file=%s", id.ID("filter"), netdev, outputFile),
		}
	}
}

// WithNIC changes the default NIC device QEMU emulates from e1000 to the given value.
func WithNIC(nic NIC) Opt {
	return func(netdev string, id *qemu.IDAllocator, opts *Options) []string {
		opts.NIC = nic
		return nil
	}
}

// WithMAC assigns a MAC address to the guest interface.
func WithMAC(mac net.HardwareAddr) Opt {
	return func(netdev string, id *qemu.IDAllocator, opts *Options) []string {
		if mac != nil {
			opts.MAC = mac
		}
		return nil
	}
}

// InterVM is a Device that can connect multiple QEMU VMs to each other.
//
// InterVM uses the QEMU socket mechanism to connect multiple VMs with a simple
// unix domain socket.
type InterVM struct {
	socket string

	// numVMs must be atomically accessed so VMs can be started in parallel
	// in goroutines.
	numVMs uint32
}

// NewInterVM creates a new QEMU network between QEMU VMs.
//
// The network is closed from the world and only between the QEMU VMs.
func NewInterVM() *InterVM {
	dir, err := os.MkdirTemp("", "intervm-")
	if err != nil {
		panic(err)
	}

	return &InterVM{
		socket: filepath.Join(dir, "intervm.socket"),
	}
}

// NewVM returns a Device that can be used with a new QEMU VM.
func (n *InterVM) NewVM(nopts ...Opt) qemu.Fn {
	if n == nil {
		return nil
	}

	newNum := atomic.AddUint32(&n.numVMs, 1)
	num := newNum - 1

	return func(alloc *qemu.IDAllocator, qopts *qemu.Options) error {
		devID := alloc.ID("vm")

		opts := Options{
			// Default NIC.
			NIC: NICE1000,

			// MAC for the virtualized NIC.
			//
			// This is from the range of locally administered address ranges.
			MAC: net.HardwareAddr{0xe, 0, 0, 0, 0, byte(num)},
		}
		var args []string
		for _, opt := range nopts {
			args = append(args, opt(devID, alloc, &opts)...)
		}
		args = append(args, "-device", fmt.Sprintf("%s,netdev=%s,mac=%s", opts.NIC, devID, opts.MAC))

		if num != 0 {
			args = append(args, "-netdev", fmt.Sprintf("stream,id=%s,server=false,addr.type=unix,addr.path=%s", devID, n.socket))
		} else {
			args = append(args, "-netdev", fmt.Sprintf("stream,id=%s,server=true,addr.type=unix,addr.path=%s", devID, n.socket))
		}
		qopts.AppendQEMU(args...)
		return nil
	}
}

// IPv4HostNetwork provides QEMU user-mode networking to the host.
//
// Net must be an IPv4 network.
//
// Default NIC is e1000, with a MAC address of 0e:00:00:00:00:01.
func IPv4HostNetwork(ipnet *net.IPNet, nopts ...Opt) qemu.Fn {
	return func(alloc *qemu.IDAllocator, qopts *qemu.Options) error {
		if ipnet.IP.To4() == nil {
			return fmt.Errorf("HostNetwork must be configured with an IPv4 address")
		}

		netdevID := alloc.ID("netdev")
		opts := Options{
			// Default NIC.
			NIC: NICE1000,

			// MAC for the virtualized NIC.
			//
			// This is from the range of locally administered address ranges.
			MAC: net.HardwareAddr{0xe, 0, 0, 0, 0, 1},
		}

		var args []string
		for _, opt := range nopts {
			args = append(args, opt(netdevID, alloc, &opts)...)
		}
		args = append(args,
			"-device", fmt.Sprintf("%s,netdev=%s,mac=%s", opts.NIC, netdevID, opts.MAC),
			"-netdev", fmt.Sprintf("user,id=%s,net=%s,dhcpstart=%s,ipv6=off", netdevID, ipnet, nthIP(ipnet, 8)),
		)
		qopts.AppendQEMU(args...)
		return nil
	}
}

func inc(ip net.IP) {
	for j := len(ip) - 1; j >= 0; j-- {
		ip[j]++
		if ip[j] > 0 {
			break
		}
	}
}

func nthIP(nt *net.IPNet, n int) net.IP {
	ip := make(net.IP, net.IPv4len)
	copy(ip, nt.IP.To4())
	for i := 0; i < n; i++ {
		inc(ip)
	}
	if !nt.Contains(ip) {
		return nil
	}
	return ip
}

// Copyright 2018 the u-root Authors. All rights reserved
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package qemu

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"syscall"

	"github.com/creack/pty"
	"github.com/hugelgupf/vmtest/internal/eventchannel"
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
		switch opts.Arch() {
		case ArchArm:
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

type ptmClosedErrorConverter struct {
	r io.Reader
}

// "read /dev/ptmx: input/output error" error occufs on Linux while reading
// from the ptm after the pts is closed.
var ptmClosed = os.PathError{
	Op:   "read",
	Path: "/dev/ptmx",
	Err:  syscall.EIO,
}

func (c ptmClosedErrorConverter) Read(p []byte) (int, error) {
	n, err := c.r.Read(p)
	var perr *os.PathError
	if errors.As(err, &perr) && *perr == ptmClosed {
		return n, io.EOF
	}
	return n, err
}

// ErrEventChannelMissingDoneEvent is returned when the final event channel
// event is not received.
var ErrEventChannelMissingDoneEvent = errors.New("never received the final event channel event (did you call Close() on the guest event channel emitter?)")

// EventChannel adds a virtio-serial-backed channel between host and guest to
// send JSON events (T).
//
// Use guest.SerialEventChannel with the same name to get access to the emitter
// in the guest.
//
// Guest events will be sent on the supplied channel. The channel will be
// closed when the guest exits or indicates that no more events are coming. If
// the guest exits without indicating that no more events are coming, the VM
// exit will return an error. (guest.SerialEventChannel.Close emits this "done"
// event.)
//
// If the channel is blocking, guest event processing is blocked as well.
func EventChannel[T any](name string, events chan<- T) Fn {
	return func(alloc *IDAllocator, opts *Options) error {
		pipeID := alloc.ID("pipe")

		ptm, pts, err := pty.Open()
		if err != nil {
			return err
		}
		fd := opts.AddFile(pts)
		opts.AppendQEMU(
			"-device", "virtio-serial",
			"-device", fmt.Sprintf("virtserialport,chardev=%s,name=%s", pipeID, name),
			"-chardev", fmt.Sprintf("pipe,id=%s,path=/proc/self/fd/%d", pipeID, fd),
		)

		var gotDone bool
		opts.Tasks = append(opts.Tasks, WaitVMStarted(func(ctx context.Context, n *Notifications) error {
			// Close ptm if it isn't already closed due to the VM
			// exiting.
			defer ptm.Close()

			// Close write-end on parent side.
			pts.Close()

			err := eventchannel.ProcessJSONByLine[eventchannel.Event[T]](ptmClosedErrorConverter{ptm}, func(c eventchannel.Event[T]) {
				switch c.GuestAction {
				case eventchannel.ActionGuestEvent:
					events <- c.Actual

				case eventchannel.ActionDone:
					close(events)
					gotDone = true
				}
			})
			if err != nil {
				if !gotDone {
					close(events)
				}
				return err
			}
			if !gotDone {
				close(events)
				return ErrEventChannelMissingDoneEvent
			}
			return nil
		}))
		return nil
	}
}

// EventChannelCallback adds a virtio-serial-backed channel between host and
// guest to send JSON events (T).
//
// Use guest.SerialEventChannel with the same name to get access to the emitter
// in the guest.
//
// When a guest event occurs, the callback is called.
func EventChannelCallback[T any](name string, callback func(T)) Fn {
	ch := make(chan T)
	return func(alloc *IDAllocator, opts *Options) error {
		opts.Tasks = append(opts.Tasks, func(ctx context.Context, n *Notifications) error {
			for {
				select {
				case <-ctx.Done():
					return ctx.Err()

				case e, ok := <-ch:
					if !ok {
						return nil
					}
					callback(e)
				}
			}
		})
		return EventChannel[T](name, ch)(alloc, opts)
	}
}

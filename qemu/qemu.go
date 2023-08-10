// Copyright 2018 the u-root Authors. All rights reserved
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package qemu provides a Go API for starting QEMU VMs.
//
// qemu is mainly suitable for running QEMU-based integration tests.
//
// The environment variable `VMTEST_QEMU` overrides the path to QEMU and the
// first few arguments. For example:
//
//	VMTEST_QEMU='qemu-system-x86_64 -L . -m 4096 -enable-kvm'
//
// Other environment variables:
//
//	VMTEST_QEMU_ARCH (used when GuestArch is empty or GuestArchUseEnvv is set)
//	VMTEST_KERNEL (used when Options.Kernel is empty)
//	VMTEST_INITRAMFS (used when Options.Initramfs is empty)
package qemu

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/Netflix/go-expect"
	"golang.org/x/exp/slices"
	"golang.org/x/sync/errgroup"
)

// ErrKernelRequiredForArgs is returned when KernelArgs is populated but Kernel is empty.
var ErrKernelRequiredForArgs = errors.New("KernelArgs can only be used when Kernel is also specified due to how QEMU bootloader works")

// ErrNoGuestArch is returned when neither GuestArch nor VMTEST_QEMU_ARCH are set.
var ErrNoGuestArch = errors.New("no QEMU guest architecture specified -- guest arch is required to decide some QEMU command-line arguments")

// ErrUnsupportedGuestArch is returned when an unsupported guest architecture value is used.
var ErrUnsupportedGuestArch = errors.New("unsupported QEMU guest architecture specified -- guest arch is required to decide some QEMU command-line arguments")

// GuestArch is the QEMU guest architecture.
type GuestArch string

const (
	GuestArchUseEnvv GuestArch = ""
	GuestArchX8664   GuestArch = "x86_64"
	GuestArchI386    GuestArch = "i386"
	GuestArchAarch64 GuestArch = "aarch64"
	GuestArchArm     GuestArch = "arm"
)

// SupportedGuestArches are the supported guest architecture values.
var SupportedGuestArches = []GuestArch{
	GuestArchX8664,
	GuestArchI386,
	GuestArchAarch64,
	GuestArchArm,
}

// Valid returns whether the guest arch is a supported guest arch value.
func (g GuestArch) Valid() bool {
	return slices.Contains(SupportedGuestArches, g)
}

// Set implements ArchFn for GuestArch.
func (g GuestArch) Setup(alloc *IDAllocator, opts *Options) error {
	return nil
}

// Arch returns the guest architecture.
func (g GuestArch) Arch() GuestArch {
	if g == GuestArchUseEnvv {
		g = GuestArch(os.Getenv("VMTEST_QEMU_ARCH"))
	}
	return g
}

// Fn is a QEMU configuration option supplied to Start or OptionsFor.
//
// Fns rely on a QEMU architecture already having been determined.
type Fn func(*IDAllocator, *Options) error

// ArchFn is a Fn that can modify Options and set the architecture. It runs before any other Fn.
type ArchFn interface {
	Setup(*IDAllocator, *Options) error
	Arch() GuestArch
}

// WithQEMUCommand sets a QEMU command. It's expected to provide a QEMU binary
// and optionally some arguments.
//
// cmd may contain additional QEMU args, such as "qemu-system-x86_64 -enable-kvm -m 1G".
// They will be appended to the command-line.
func WithQEMUCommand(cmd string) Fn {
	return func(alloc *IDAllocator, opts *Options) error {
		opts.QEMUCommand = cmd
		return nil
	}
}

// WithKernel sets the path to the kernel binary.
func WithKernel(kernel string) Fn {
	return func(alloc *IDAllocator, opts *Options) error {
		opts.Kernel = kernel
		return nil
	}
}

// WithInitramfs sets the path to the initramfs.
func WithInitramfs(initramfs string) Fn {
	return func(alloc *IDAllocator, opts *Options) error {
		opts.Initramfs = initramfs
		return nil
	}
}

// WithAppendKernel appends kernel arguments.
func WithAppendKernel(args ...string) Fn {
	return func(alloc *IDAllocator, opts *Options) error {
		opts.AppendKernel(strings.Join(args, " "))
		return nil
	}
}

// WithSerialOutput writes serial output to w as well.
func WithSerialOutput(w ...io.WriteCloser) Fn {
	return func(alloc *IDAllocator, opts *Options) error {
		opts.SerialOutput = append(opts.SerialOutput, w...)
		return nil
	}
}

// WithVMTimeout is a timeout for the QEMU guest subprocess.
func WithVMTimeout(timeout time.Duration) Fn {
	return func(alloc *IDAllocator, opts *Options) error {
		opts.VMTimeout = timeout
		return nil
	}
}

// WithTask adds a goroutine running alongside the guest.
//
// Task goroutines are started right before the guest is started.
//
// A task is expected to exit either when ctx is canceled or when the QEMU
// subprocess exits. When the context is canceled, the QEMU subprocess is
// expected to exit as well, and when the QEMU subprocess exits, the context is
// canceled.
func WithTask(t ...Task) Fn {
	return func(alloc *IDAllocator, opts *Options) error {
		opts.Tasks = append(opts.Tasks, t...)
		return nil
	}
}

// OptionsFor evaluates the given config functions and returns an Options object.
func OptionsFor(archFn ArchFn, fns ...Fn) (*Options, error) {
	alloc := NewIDAllocator()
	o := &Options{
		QEMUCommand: os.Getenv("VMTEST_QEMU"),
		Kernel:      os.Getenv("VMTEST_KERNEL"),
		Initramfs:   os.Getenv("VMTEST_INITRAMFS"),
		// Disable graphics by default.
		QEMUArgs: []string{"-nographic"},
	}

	if err := o.setArch(archFn.Arch()); err != nil {
		return nil, err
	}
	if err := archFn.Setup(alloc, o); err != nil {
		return nil, err
	}

	for _, f := range fns {
		if err := f(alloc, o); err != nil {
			return nil, err
		}
	}
	return o, nil
}

// Start starts a VM with the given configuration.
//
// SerialOutput will be relayed only if VM.Wait is also called some time after
// the VM starts.
func Start(arch ArchFn, fns ...Fn) (*VM, error) {
	return StartContext(context.Background(), arch, fns...)
}

// StartContext starts a VM with the given configuration and with the given context.
//
// When the context is done, the QEMU subprocess will be killed and all
// associated goroutines cleaned up as long as VM.Wait() is called.
func StartContext(ctx context.Context, arch ArchFn, fns ...Fn) (*VM, error) {
	o, err := OptionsFor(arch, fns...)
	if err != nil {
		return nil, err
	}
	return o.Start(ctx)
}

// Options are VM start-up parameters.
type Options struct {
	// arch is the QEMU architecture used.
	//
	// Some device decisions are made based on the architecture.
	// If empty, VMTEST_QEMU_ARCH env var will be used.
	arch GuestArch

	// QEMUCommand is QEMU binary to invoke and some additonal args.
	//
	// If empty, the VMTEST_QEMU env var will be used.
	QEMUCommand string

	// Path to the kernel to boot.
	//
	// If empty, VMTEST_KERNEL env var will be used.
	Kernel string

	// Path to the initramfs.
	//
	// If empty, VMTEST_INITRAMFS env var will be used.
	Initramfs string

	// Extra kernel command-line arguments.
	KernelArgs string

	// Where to send serial output.
	SerialOutput []io.WriteCloser

	// Tasks are goroutines running alongside the guest.
	//
	// Task goroutines are started right before the guest is started.
	//
	// A task is expected to exit either when ctx is canceled or when the
	// QEMU subprocess exits. When the context is canceled, the QEMU
	// subprocess is expected to exit as well, and when the QEMU subprocess
	// exits, the context is canceled.
	Tasks []Task

	// Additional QEMU cmdline arguments.
	QEMUArgs []string

	// VMTimeout is a timeout for the QEMU subprocess.
	VMTimeout time.Duration

	// ExtraFiles are extra files passed to QEMU on start.
	ExtraFiles []*os.File
}

// AddFile adds the file to the QEMU process and returns the FD it will be in
// the child process.
func (o *Options) AddFile(f *os.File) int {
	o.ExtraFiles = append(o.ExtraFiles, f)

	// 0, 1, 2 used for stdin/out/err.
	return len(o.ExtraFiles) + 2
}

// Task is a task running alongside the guest.
//
// A task is expected to exit either when ctx is canceled or when the QEMU
// subprocess exits.
type Task func(ctx context.Context, n *Notifications) error

// WaitVMStarted waits until the VM has started before starting t, or never
// starts t if context is canceled before the VM is started.
func WaitVMStarted(t Task) Task {
	return func(ctx context.Context, n *Notifications) error {
		// Wait until VM starts or exit if it never does.
		select {
		case <-n.VMStarted:
		case <-ctx.Done():
			return nil
		}
		return t(ctx, n)
	}
}

// Notifications gives tasks the option to wait for certain VM events.
//
// Tasks must not be required to listen on notifications; there must be no
// blocking channel I/O.
type Notifications struct {
	// VMStarted will be closed when the VM is started.
	VMStarted chan struct{}

	// VMExited will receive exactly 1 event when the VM exits and then be closed.
	VMExited chan error
}

func newNotifications() *Notifications {
	return &Notifications{
		VMStarted: make(chan struct{}),
		VMExited:  make(chan error, 1),
	}
}

// GuestArch returns the guest architecture.
func (o *Options) GuestArch() GuestArch {
	return o.arch
}

// Start starts a QEMU VM.
func (o *Options) Start(ctx context.Context) (*VM, error) {
	cmdline, err := o.Cmdline()
	if err != nil {
		return nil, err
	}

	var eopt []expect.ConsoleOpt
	for _, serial := range o.SerialOutput {
		eopt = append(eopt, expect.WithStdout(serial), expect.WithCloser(serial))
	}
	c, err := expect.NewConsole(eopt...)
	if err != nil {
		return nil, err
	}

	var cancel context.CancelFunc
	if o.VMTimeout != 0 {
		ctx, cancel = context.WithTimeout(ctx, o.VMTimeout)
	} else {
		ctx, cancel = context.WithCancel(ctx)
	}
	vm := &VM{
		Options: o,
		Console: c,
		cmdline: cmdline,
		cancel:  cancel,
	}
	for _, task := range o.Tasks {
		// Capture the var... Go stuff.
		task := task
		n := newNotifications()
		vm.wg.Go(func() error {
			return task(ctx, n)
		})
		vm.notifs = append(vm.notifs, n)
	}

	cmd := exec.CommandContext(ctx, cmdline[0], cmdline[1:]...)
	cmd.Stdin = c.Tty()
	cmd.Stdout = c.Tty()
	cmd.Stderr = c.Tty()
	cmd.ExtraFiles = o.ExtraFiles
	if err := cmd.Start(); err != nil {
		// Cancel tasks.
		cancel()
		// Wait for tasks to exit. Some day we'll report their errors
		// with errors.Join.
		_ = vm.wg.Wait()
		return nil, err
	}
	vm.notifs.VMStarted()

	// Close tty in parent, so that when child exits, the last reference to
	// it is gone and Console.Expect* calls automatically exit.
	c.Tty().Close()

	vm.cmd = cmd
	return vm, nil
}

func (o *Options) setArch(arch GuestArch) error {
	if len(arch) == 0 {
		return ErrNoGuestArch
	}
	if !arch.Valid() {
		return fmt.Errorf("%w: %s", ErrUnsupportedGuestArch, arch)
	}
	o.arch = arch
	return nil
}

// AppendKernel appends to kernel args.
func (o *Options) AppendKernel(s ...string) {
	if len(s) == 0 {
		return
	}
	t := strings.Join(s, " ")
	if len(o.KernelArgs) == 0 {
		o.KernelArgs = t
	} else {
		o.KernelArgs += " " + t
	}
}

// AppendQEMU appends args to the QEMU command line.
func (o *Options) AppendQEMU(s ...string) {
	o.QEMUArgs = append(o.QEMUArgs, s...)
}

// Cmdline returns the command line arguments used to start QEMU. These
// arguments are derived from the given QEMU struct.
func (o *Options) Cmdline() ([]string, error) {
	var args []string

	// QEMU binary + initial args (may have been supplied via VMTEST_QEMU).
	args = append(args, strings.Fields(o.QEMUCommand)...)

	// Add user / configured args.
	args = append(args, o.QEMUArgs...)

	if len(o.Kernel) > 0 {
		args = append(args, "-kernel", o.Kernel)
		if len(o.KernelArgs) != 0 {
			args = append(args, "-append", o.KernelArgs)
		}
	} else if len(o.KernelArgs) != 0 {
		return nil, ErrKernelRequiredForArgs
	}

	if len(o.Initramfs) != 0 {
		args = append(args, "-initrd", o.Initramfs)
	}

	return args, nil
}

// VM is a running QEMU virtual machine.
type VM struct {
	// Console provides in/output to the QEMU subprocess.
	Console *expect.Console

	// Options are the options that were used to start the VM.
	//
	// They are not used once the VM is started.
	Options *Options

	// cmd is the QEMU subprocess.
	cmd *exec.Cmd

	// The cmdline that the QEMU subprocess was started with.
	cmdline []string

	// State related to tasks.
	wg     errgroup.Group
	notifs notifications
	cancel func()
}

// Cmdline is the command-line the VM was started with.
func (v *VM) Cmdline() []string {
	// Maybe return a copy?
	return v.cmdline
}

// Kill kills the QEMU subprocess.
//
// Callers are still responsible for calling VM.Wait after calling kill to
// clean up task goroutines and to get remaining serial console output.
func (v *VM) Kill() error {
	return v.cmd.Process.Kill()
}

// Signal signals the QEMU subprocess.
//
// Callers are still responsible for calling VM.Wait if the subprocess exits
// due to this signal to clean up task goroutines and to get remaining serial
// console output.
func (v *VM) Signal(sig os.Signal) error {
	return v.cmd.Process.Signal(sig)
}

// Wait waits for the VM to exit and expects EOF from the expect console.
func (v *VM) Wait() error {
	err := v.cmd.Wait()
	v.notifs.VMExited(err)
	if _, cerr := v.Console.ExpectEOF(); cerr != nil && err == nil {
		err = cerr
	}
	v.Console.Close()

	v.cancel()
	if werr := v.wg.Wait(); werr != nil && err == nil {
		err = werr
	}
	return err
}

// CmdlineQuoted quotes any of QEMU's command line arguments containing a space
// so it is easy to copy-n-paste into a shell for debugging.
func (v *VM) CmdlineQuoted() string {
	args := make([]string, len(v.cmdline))
	for i, arg := range v.cmdline {
		if strings.ContainsAny(arg, " \t\n") {
			args[i] = fmt.Sprintf("'%s'", arg)
		} else {
			args[i] = arg
		}
	}
	return strings.Join(args, " ")
}

type notifications []*Notifications

func (n notifications) VMStarted() {
	for _, m := range n {
		close(m.VMStarted)
	}
}

func (n notifications) VMExited(err error) {
	for _, m := range n {
		m.VMExited <- err
		close(m.VMExited)
	}
}

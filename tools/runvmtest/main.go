// Copyright 2023 the u-root Authors. All rights reserved
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// runvmtest sets appropriate environment variables for the execution of a
// vmtest binary.
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"

	"dagger.io/dagger"
)

var (
	keepArtifacts = flag.Bool("keep-artifacts", false, "Keep artifacts directory available for further local tests")
)

func main() {
	if err := run(); err != nil {
		log.Fatalf("Error: %v", err)
	}
}

type TestEnvConfig struct {
	KernelContainer string
	KernelPath      string
	QEMUContainer   string
	QEMUCmd         string
	QEMUPath        string
	BIOSPath        string
}

func (tc *TestEnvConfig) RegisterFlags(f *flag.FlagSet) {
	// Note the default value is whatever is in tc already.
	f.StringVar(&tc.KernelContainer, "kernel-container", tc.KernelContainer, "Container to use for kernel files")
	f.StringVar(&tc.KernelPath, "kernel-path", tc.KernelPath, "Path where to find the kernel image")
	f.StringVar(&tc.QEMUContainer, "qemu-container", tc.QEMUContainer, "Container to use for QEMU files")
	f.StringVar(&tc.QEMUCmd, "qemu-cmd", tc.QEMUCmd, "QEMU command with platform specific flags")
	f.StringVar(&tc.QEMUPath, "qemu-path", tc.QEMUPath, "Path where to find the QEMU binary")
	f.StringVar(&tc.BIOSPath, "bios-path", tc.BIOSPath, "Path where to find the BIOS image")
}

var configs = map[string]TestEnvConfig{
	"amd64": {
		KernelContainer: "ghcr.io/hugelgupf/vmtest/kernel-amd64:main",
		KernelPath:      "/bzImage",
		QEMUContainer:   "ghcr.io/hugelgupf/vmtest/qemu:main",
		QEMUCmd:         "qemu-system-x86_64 -L %s -m 1G",
		QEMUPath:        "/zqemu/bin/qemu-system-x86_64",
		BIOSPath:        "/zqemu/pc-bios",
	},
	"arm": {
		KernelContainer: "ghcr.io/hugelgupf/vmtest/kernel-arm:main",
		KernelPath:      "/zImage",
		QEMUContainer:   "ghcr.io/hugelgupf/vmtest/qemu:main",
		QEMUCmd:         "qemu-system-arm -M virt,highmem=off -L %s",
		QEMUPath:        "/zqemu/bin/qemu-system-arm",
		BIOSPath:        "/zqemu/pc-bios",
	},
	"arm64": {
		KernelContainer: "ghcr.io/hugelgupf/vmtest/kernel-arm64:main",
		KernelPath:      "/Image",
		QEMUContainer:   "ghcr.io/hugelgupf/vmtest/qemu:main",
		QEMUCmd:         "qemu-system-aarch64 -machine virt -cpu max -m 1G -L %s",
		QEMUPath:        "/zqemu/bin/qemu-system-aarch64",
		BIOSPath:        "/zqemu/pc-bios",
	},
}

func defaultConfig() TestEnvConfig {
	arch := os.Getenv("VMTEST_ARCH")
	if c, ok := configs[arch]; ok {
		return c
	}
	if c, ok := configs[runtime.GOARCH]; ok {
		return c
	}
	// On other architectures, user has to provide all values via flags.
	return TestEnvConfig{}
}

func run() error {
	config := defaultConfig()
	config.RegisterFlags(flag.CommandLine)
	flag.Parse()

	if flag.NArg() < 2 {
		return fmt.Errorf("too few arguments: usage: `%s -- ./test-to-run`", os.Args[0])
	}

	ctx := context.Background()
	client, err := dagger.Connect(ctx, dagger.WithLogOutput(os.Stdout))
	if err != nil {
		return fmt.Errorf("unable to connect to client: %w", err)
	}
	defer client.Close()

	artifacts := client.
		Container().
		From(config.QEMUContainer).
		WithFile(config.KernelPath, client.Container().From(config.KernelContainer).File(config.KernelPath)).
		Directory("/")

	return runNatively(ctx, artifacts, config.KernelPath, config.QEMUCmd, flag.Args())
}

func runNatively(ctx context.Context, artifacts *dagger.Directory, kpath, qemuCmd string, args []string) error {
	tmp, err := os.MkdirTemp(".", "ci-testing")
	if err != nil {
		return fmt.Errorf("unable to create tmp dir: %w", err)
	}
	if !*keepArtifacts {
		defer os.RemoveAll(tmp)
	}

	if ok, err := artifacts.Export(ctx, tmp); !ok || err != nil {
		return fmt.Errorf("failed artifact export: %w", err)
	}

	tmp, err = filepath.Abs(tmp)
	if err != nil {
		return fmt.Errorf("could not retrieve absolute path: %w", err)
	}

	kpath = filepath.Join(tmp, kpath)
	qemuCmd = fmt.Sprintf(qemuCmd, filepath.Join(tmp, "zqemu", "pc-bios"))

	// Rather than adding the QEMU Cmd to PATH in cmd.Env,
	// we are doing this because args[0] can be qemu, and if that's the case,
	// exec.Command does not evaluate the PATH in cmd.Env, but instead the one in the current environment.
	// The PATH will also be restored after the program exits.
	p := os.Getenv("PATH")
	if err := os.Setenv("PATH", fmt.Sprintf("%s:%s", p, filepath.Join(tmp, "zqemu", "bin"))); err != nil {
		return fmt.Errorf("failed to update PATH: %w", err)
	}
	defer os.Setenv("PATH", p)

	cmd := exec.Command(args[0], args[1:]...)
	cmd.Env = append(
		os.Environ(),
		fmt.Sprintf("VMTEST_KERNEL=%s", kpath),
		fmt.Sprintf("VMTEST_QEMU=%s", qemuCmd),
	)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if *keepArtifacts {
		defer func() {
			fmt.Println("\nTo run another test using the same artifacts:")

			qemuCmd = filepath.Join(tmp, "qemu", "bin") + "/" + qemuCmd
			fmt.Printf("VMTEST_KERNEL=%s VMTEST_QEMU=%q ...\n", kpath, qemuCmd)
		}()
	}

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed execution: %w", err)
	}
	return nil
}

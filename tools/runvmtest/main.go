// Copyright 2023 the u-root Authors. All rights reserved
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// runvmtest sets VMTEST_QEMU and VMTEST_KERNEL (if not already set) with
// binaries downloaded from Docker images, then executes a command.
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
	"strings"
	"text/template"

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
	f.StringVar(&tc.QEMUCmd, "qemu-cmd", tc.QEMUCmd, "QEMU command with platform-specific flags (template variables {{.QEMUBin}} and {{.BIOSPath}} available)")
	f.StringVar(&tc.QEMUPath, "qemu-path", tc.QEMUPath, "Path where to find the QEMU binary")
	f.StringVar(&tc.BIOSPath, "bios-path", tc.BIOSPath, "Path where to find the BIOS image")
}

var configs = map[string]TestEnvConfig{
	"amd64": {
		KernelContainer: "ghcr.io/hugelgupf/vmtest/kernel-amd64:main",
		KernelPath:      "/bzImage",
		QEMUContainer:   "ghcr.io/hugelgupf/vmtest/qemu:main",
		QEMUCmd:         "{{.QEMUBin}} -L {{.BIOSPath}} -m 1G",
		QEMUPath:        "/zqemu/bin/qemu-system-x86_64",
		BIOSPath:        "/zqemu/pc-bios",
	},
	"arm": {
		KernelContainer: "ghcr.io/hugelgupf/vmtest/kernel-arm:main",
		KernelPath:      "/zImage",
		QEMUContainer:   "ghcr.io/hugelgupf/vmtest/qemu:main",
		QEMUCmd:         "{{.QEMUBin}} -M virt,highmem=off -L {{.BIOSPath}}",
		QEMUPath:        "/zqemu/bin/qemu-system-arm",
		BIOSPath:        "/zqemu/pc-bios",
	},
	"arm64": {
		KernelContainer: "ghcr.io/hugelgupf/vmtest/kernel-arm64:main",
		KernelPath:      "/Image",
		QEMUContainer:   "ghcr.io/hugelgupf/vmtest/qemu:main",
		QEMUCmd:         "{{.QEMUBin}} -machine virt -cpu max -m 1G -L {{.BIOSPath}}",
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

	return runNatively(ctx, artifacts, &config, flag.Args())
}

func runNatively(ctx context.Context, artifacts *dagger.Directory, config *TestEnvConfig, args []string) error {
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

	kpath := filepath.Join(tmp, config.KernelPath)
	sub := struct {
		QEMUBin  string
		BIOSPath string
	}{
		QEMUBin:  filepath.Join(tmp, config.QEMUPath),
		BIOSPath: filepath.Join(tmp, config.BIOSPath),
	}
	qemuTemplate, err := template.New("qemu").Parse(config.QEMUCmd)
	if err != nil {
		return fmt.Errorf("invalid QEMU command template: %w", err)
	}
	var qemuCmd strings.Builder
	if err := qemuTemplate.Execute(&qemuCmd, sub); err != nil {
		return fmt.Errorf("failed to substitute QEMU command template variables: %w", err)
	}

	cmd := exec.Command(args[0], args[1:]...)
	cmd.Env = os.Environ()
	if os.Getenv("VMTEST_KERNEL") == "" {
		cmd.Env = append(cmd.Env, fmt.Sprintf("VMTEST_KERNEL=%s", kpath))
	}
	if os.Getenv("VMTEST_QEMU") == "" {
		cmd.Env = append(cmd.Env, fmt.Sprintf("VMTEST_QEMU=%s", qemuCmd.String()))
	}
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if *keepArtifacts {
		defer func() {
			fmt.Println("\nTo run another test using the same artifacts:")

			fmt.Printf("VMTEST_KERNEL=%s VMTEST_QEMU=%q ...\n", kpath, qemuCmd.String())
		}()
	}

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed execution: %w", err)
	}
	return nil
}

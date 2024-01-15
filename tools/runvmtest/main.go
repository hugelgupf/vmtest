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
	"os/signal"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"text/template"

	"dagger.io/dagger"
	"gopkg.in/yaml.v3"
)

var (
	keepArtifacts = flag.Bool("keep-artifacts", false, "Keep artifacts directory available after exiting (alias -k)")
	configFile    = flag.String("config", "", "Path to YAML config file")
	artifactsDir  = flag.String("artifacts-dir", "", "Directory to store artifacts in, will be created if not exist (default: temp dir)")
)

func init() {
	flag.BoolVar(keepArtifacts, "k", false, "Keep artifacts directory available after exiting")
	flag.StringVar(artifactsDir, "d", "", "Directory to store artifacts in, will be created if not exist (default: temp dir)")
}

func main() {
	if err := run(); err != nil {
		log.Fatalf("Error: %v", err)
	}
}

// EnvConfig is a map of env var name -> variable config.
type EnvConfig map[string]EnvVar

// Config is a map of VMTEST_ARCH -> config for env vars to set up.
type Config map[string]EnvConfig

// EnvVar is the configuration for a template & files to fill in an environment
// variable.
type EnvVar struct {
	// Container is the name of the container to pull files from.
	Container string

	// Template uses text/template syntax and is evaluated to become the env var.
	//
	// {{.$name}} can be used to refer to files extracted from the
	// container, where $name is the key to one of the Files / Directories
	// maps.
	Template string

	// Map of template variable name -> path in container
	Files map[string]string

	// Map of template variable name -> path in container
	Directories map[string]string
}

var defaultConfig = Config{
	"amd64": map[string]EnvVar{
		"VMTEST_KERNEL": {
			Container: "ghcr.io/hugelgupf/vmtest/kernel-amd64:main",
			Template:  "{{.bzImage}}",
			Files:     map[string]string{"bzImage": "/bzImage"},
		},
		"VMTEST_QEMU": {
			Container:   "ghcr.io/hugelgupf/vmtest/qemu:main",
			Template:    "{{.qemu}}/bin/qemu-system-x86_64 -L {{.qemu}}/pc-bios -m 1G",
			Directories: map[string]string{"qemu": "/zqemu"},
		},
	},
	"arm": map[string]EnvVar{
		"VMTEST_KERNEL": {
			Container: "ghcr.io/hugelgupf/vmtest/kernel-arm:main",
			Template:  "{{.zImage}}",
			Files:     map[string]string{"zImage": "/zImage"},
		},
		"VMTEST_QEMU": {
			Container:   "ghcr.io/hugelgupf/vmtest/qemu:main",
			Template:    "{{.qemu}}/bin/qemu-system-arm -M virt,highmem=off -L {{.qemu}}/pc-bios",
			Directories: map[string]string{"qemu": "/zqemu"},
		},
	},
	"arm64": map[string]EnvVar{
		"VMTEST_KERNEL": {
			Container: "ghcr.io/hugelgupf/vmtest/kernel-arm64:main",
			Template:  "{{.Image}}",
			Files:     map[string]string{"Image": "/Image"},
		},
		"VMTEST_QEMU": {
			Container:   "ghcr.io/hugelgupf/vmtest/qemu:main",
			Template:    "{{.qemu}}/bin/qemu-system-aarch64 -machine virt -cpu max -m 1G -L {{.qemu}}/pc-bios",
			Directories: map[string]string{"qemu": "/zqemu"},
		},
	},
	"riscv64": map[string]EnvVar{
		"VMTEST_KERNEL": {
			Container: "ghcr.io/hugelgupf/vmtest/kernel-riscv64:main",
			Template:  "{{.Image}}",
			Files:     map[string]string{"Image": "/Image"},
		},
		"VMTEST_QEMU": {
			Container:   "ghcr.io/hugelgupf/vmtest/qemu:main",
			Template:    "{{.qemu}}/bin/qemu-system-riscv64 -M virt -cpu rv64 -m 1G -L {{.qemu}}/pc-bios -m 1G",
			Directories: map[string]string{"qemu": "/zqemu"},
		},
	},
}

func archConfig(config Config) EnvConfig {
	arch := os.Getenv("VMTEST_ARCH")
	if c, ok := config[arch]; ok {
		return c
	}
	if c, ok := config[runtime.GOARCH]; ok {
		return c
	}
	// On other architectures, user has to provide all values via flags.
	return EnvConfig{}
}

func findConfigFile(name string) (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for dir != "/" {
		p := filepath.Join(dir, name)
		if _, err := os.Stat(p); err == nil {
			return p, nil
		}
		dir = filepath.Dir(dir)
	}
	return "", fmt.Errorf("could not find %s in current directory or any parent", name)
}

func run() error {
	flag.Parse()

	if flag.NArg() < 1 {
		return fmt.Errorf("too few arguments: usage: `%s -- ./cmd-to-run`", os.Args[0])
	}

	var configPath string
	if *configFile != "" {
		configPath = *configFile
	} else {
		configPath, _ = findConfigFile(".vmtest.yaml")
	}

	var config Config = defaultConfig
	if configPath != "" {
		b, err := os.ReadFile(configPath)
		if err != nil {
			return err
		}
		if err := yaml.Unmarshal(b, &config); err != nil {
			return fmt.Errorf("could not decode YAML config from %s: %v", configPath, err)
		}
	}
	c := archConfig(config)

	ctx := context.Background()
	client, err := dagger.Connect(ctx, dagger.WithLogOutput(os.Stdout))
	if err != nil {
		return fmt.Errorf("unable to connect to client: %w", err)
	}
	defer client.Close()

	return runNatively(ctx, client, c, flag.Args())
}

func runNatively(ctx context.Context, client *dagger.Client, config EnvConfig, args []string) error {
	var tmpDir string

	if !*keepArtifacts {
		c := make(chan os.Signal, 1)
		defer close(c)

		signal.Notify(c, os.Interrupt)
		defer signal.Stop(c)

		var wg sync.WaitGroup
		wg.Add(1)
		go func() {
			defer wg.Done()
			select {
			case <-c:
				os.RemoveAll(tmpDir)

			case <-ctx.Done():
				return
			}
		}()

		defer wg.Wait()
	}

	// Cancel before wg.Wait(), so goroutine can exit.
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	var err error
	if *artifactsDir != "" {
		tmpDir = *artifactsDir
		if err := os.MkdirAll(tmpDir, 0o700); err != nil {
			return fmt.Errorf("could not create artifact directory: %v", err)
		}
	} else if tmpDir, err = os.MkdirTemp(".", "runvmtest-artifacts"); err != nil {
		return fmt.Errorf("unable to create tmp dir: %w", err)
	}
	if !*keepArtifacts {
		defer os.RemoveAll(tmpDir)
	}
	tmp, err := filepath.Abs(tmpDir)
	if err != nil {
		return fmt.Errorf("could not retrieve absolute path: %w", err)
	}

	base := client.Container()
	var envv []string
	for varName, varConf := range config {
		// Already set by caller.
		if os.Getenv(varName) != "" {
			continue
		}

		files := make(map[string]string)
		for templateName, file := range varConf.Files {
			base = base.WithFile(file, client.Container().From(varConf.Container).File(file))
			files[templateName] = filepath.Join(tmp, file)
		}
		for templateName, dir := range varConf.Directories {
			base = base.WithDirectory(dir, client.Container().From(varConf.Container).Directory(dir))
			files[templateName] = filepath.Join(tmp, dir)
		}

		tmpl, err := template.New(varName).Parse(varConf.Template)
		if err != nil {
			return fmt.Errorf("invalid %s template: %w", varName, err)
		}
		var s strings.Builder
		if err := tmpl.Execute(&s, files); err != nil {
			return fmt.Errorf("failed to substitute %s template variables: %w", varName, err)
		}
		envv = append(envv, varName+"="+s.String())
	}
	artifacts := base.Directory("/")

	if ok, err := artifacts.Export(ctx, tmp); !ok || err != nil {
		return fmt.Errorf("failed artifact export: %w", err)
	}

	cmd := exec.Command(args[0], args[1:]...)
	cmd.Env = append(os.Environ(), envv...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if *keepArtifacts {
		defer func() {
			fmt.Println("\nTo run another test using the same artifacts:")

			fmt.Printf("%s ...\n", strings.Join(envv, " "))
		}()
	}

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed execution: %w", err)
	}
	return nil
}

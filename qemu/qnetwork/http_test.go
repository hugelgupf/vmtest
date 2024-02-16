// Copyright 2023 the u-root Authors. All rights reserved
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package qnetwork

import (
	"errors"
	"fmt"
	"net"
	"net/http"
	"os/exec"
	"testing"
	"testing/fstest"
	"time"

	"github.com/hugelgupf/vmtest/qemu"
	"github.com/hugelgupf/vmtest/qemu/quimage"
	"github.com/u-root/mkuimage/uimage"
)

func TestHTTPTask(t *testing.T) {
	fs := fstest.MapFS{
		"foobar": &fstest.MapFile{
			Data:    []byte("Hello, world!"),
			Mode:    0o777,
			ModTime: time.Now(),
		},
	}

	// Serve HTTP on the host on a random port.
	http.Handle("/", http.FileServer(http.FS(fs)))
	ln, err := net.Listen("tcp", ":0")
	if err != nil {
		t.Fatal(err)
	}
	port := ln.Addr().(*net.TCPAddr).Port

	s := &http.Server{}
	vm := qemu.StartT(
		t,
		"vm",
		qemu.ArchUseEnvv,
		quimage.WithUimageT(t,
			uimage.WithInit("init"),
			uimage.WithUinit("httpdownload", "-url", fmt.Sprintf("http://192.168.0.2:%d/foobar", port)),
			uimage.WithBusyboxCommands(
				"github.com/u-root/u-root/cmds/core/init",
				"github.com/u-root/u-root/cmds/core/dhclient",
				"github.com/hugelgupf/vmtest/tests/cmds/httpdownload",
			),
		),
		qemu.VirtioRandom(), // dhclient needs to generate a random number.
		ServeHTTP(s, ln),
		IPv4HostNetwork("192.168.0.0/24"),
	)

	if _, err := vm.Console.ExpectString("Hello, world!"); err != nil {
		t.Errorf("Error expecting I AM HERE: %v", err)
	}

	if err := vm.Wait(); err != nil {
		t.Errorf("Error waiting for VM to exit: %v", err)
	}
}

func TestStartFailsServeHTTP(t *testing.T) {
	ln, err := net.Listen("tcp", ":0")
	if err != nil {
		t.Fatal(err)
	}
	s := &http.Server{}

	// Test that we do not block forever. Both tasks added by ServeHTTP
	// should unblock.
	_, err = qemu.Start(qemu.ArchAMD64,
		qemu.WithQEMUCommand("does-not-exist"),
		ServeHTTP(s, ln),
	)
	if !errors.Is(err, exec.ErrNotFound) {
		t.Fatalf("Failed to start VM: %v", err)
	}
}

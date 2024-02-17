// Copyright 2023 the u-root Authors. All rights reserved
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package qnetwork

import (
	"errors"
	"net"
	"net/http"
	"os/exec"
	"testing"

	"github.com/hugelgupf/vmtest/qemu"
)

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

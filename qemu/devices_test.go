// Copyright 2018 the u-root Authors. All rights reserved
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package qemu

import (
	"testing"
)

func TestIDAllocator(t *testing.T) {
	tc := []struct {
		in   string
		want string
	}{
		{in: "pipe", want: "pipe0"},
		{in: "pipe", want: "pipe1"},
		{in: "pipe0", want: "pipe2"},
		{in: "pipe45", want: "pipe3"},
		{in: "0pipe34", want: "0pipe0"},
		{in: "pip", want: "pip0"},
		{in: "id", want: "id0"},
		{in: "pip", want: "pip1"},
	}
	a := NewIDAllocator()
	for _, c := range tc {
		got := a.ID(c.in)
		if got != c.want {
			t.Errorf("ID(%s) = %s, want %s", c.in, got, c.want)
		}
	}
}

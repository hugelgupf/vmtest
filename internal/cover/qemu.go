// Copyright 2023 the u-root Authors. All rights reserved
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package cover adds an internal coverage function.
package cover

import (
	"slices"

	"github.com/u-root/mkuimage/uimage"
)

// WithCoverInstead removes a command already added to options and instead adds
// it back with WithCoveredCommands.
//
// This allows a cmd to be built as part of busybox for regular vmtest users,
// but with coverage for vmtest's own tests.
func WithCoverInstead(cmd string) uimage.Modifier {
	return func(o *uimage.Opts) error {
		for i := range o.Commands {
			o.Commands[i].Packages = slices.DeleteFunc(o.Commands[i].Packages, func(s string) bool {
				return s == cmd
			})
		}

		return o.Apply(uimage.WithCoveredCommands(cmd))
	}
}

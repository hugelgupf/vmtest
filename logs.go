// Copyright 2018 the u-root Authors. All rights reserved
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package vmtest

import (
	"io"
	"testing"

	"github.com/u-root/u-root/pkg/testutil"
	"github.com/u-root/u-root/pkg/uio"
)

// TestLineWriter is an io.Writer that logs full lines of serial to tb.
func TestLineWriter(tb testing.TB, prefix string) io.WriteCloser {
	return uio.FullLineWriter(&testLineWriter{tb: tb, prefix: prefix})
}

type jsonStripper struct {
	uio.LineWriter
}

func (j jsonStripper) OneLine(p []byte) {
	// Poor man's JSON detector.
	if len(p) == 0 || p[0] == '{' {
		return
	}
	j.LineWriter.OneLine(p)
}

func JSONLessTestLineWriter(tb testing.TB, prefix string) io.WriteCloser {
	return uio.FullLineWriter(jsonStripper{&testLineWriter{tb: tb, prefix: prefix}})
}

// testLineWriter is an io.Writer that logs full lines of serial to tb.
type testLineWriter struct {
	tb     testing.TB
	prefix string
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

func (tsw *testLineWriter) OneLine(p []byte) {
	tsw.tb.Logf("%s %s: %s", testutil.NowLog(), tsw.prefix, string(replaceCtl(p)))
}

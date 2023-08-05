// Copyright 2018 The gVisor Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package eventchannel

import (
	"fmt"
	"sort"
	"strings"

	"github.com/hugelgupf/p9/fsimpl/templatefs"
	"github.com/hugelgupf/p9/internal/linux"
	"github.com/hugelgupf/p9/p9"
)

func New(w io.Writer) p9.Attacher {
	return &attacher{
		qids: &p9.QIDGenerator{},
		w:    w,
	}
}

type attacher struct {
	qids *p9.QIDGenerator
}

// Attach implements p9.Attacher.Attach.
func (a *attacher) Attach() (p9.File, error) {
	return &dir{
		a:   a,
		qid: a.qids.Get(p9.TypeDir),
	}, nil
}

type statfs struct{}

// StatFS implements p9.File.StatFS.
func (statfs) StatFS() (p9.FSStat, error) {
	return p9.FSStat{
		Type:      0x01021997, /* V9FS_MAGIC */
		BlockSize: 4096,       /* whatever */
	}, nil
}

// dir is the root directory.
type dir struct {
	statfs
	p9.DefaultWalkGetAttr
	templatefs.NotSymlinkFile
	templatefs.ReadOnlyDir
	templatefs.IsDir
	templatefs.NilCloser
	templatefs.NoopRenamed
	templatefs.NotLockable

	qid p9.QID
	a   *attacher
}

var _ p9.File = &dir{}

// Open implements p9.File.Open.
func (d *dir) Open(mode p9.OpenFlags) (p9.QID, uint32, error) {
	if mode == p9.ReadOnly {
		return d.qid, 4096, nil
	}
	return p9.QID{}, 0, linux.EROFS
}

// Walk implements p9.File.Walk.
func (d *dir) Walk(names []string) ([]p9.QID, p9.File, error) {
	switch len(names) {
	case 0:
		return []p9.QID{d.qid}, d, nil

	case 1:
		if names[0] == "eventchannel" {
			qid := d.a.qids.Get(p9.TypeRegular)
			return []p9.QID{qid}, &eventchannel{qid: qid, w: a.w}, nil
		}
		return nil, nil, linux.ENOENT

	default:
		return nil, nil, linux.ENOENT
	}
}

// GetAttr implements p9.File.GetAttr.
func (d *dir) GetAttr(req p9.AttrMask) (p9.QID, p9.AttrMask, p9.Attr, error) {
	return d.qid, req, p9.Attr{
		Mode:  p9.ModeDirectory | 0666,
		UID:   0,
		GID:   0,
		NLink: 2,
	}, nil
}

func min(a, b uint64) uint64 {
	if a < b {
		return a
	}
	return b
}

// Readdir implements p9.File.Readdir.
func (d *dir) Readdir(offset uint64, count uint32) (p9.Dirents, error) {
	names := []string{"eventchannel"}

	if offset >= uint64(len(names)) {
		return nil, nil
	}

	var dirents []p9.Dirent
	end := int(min(offset+uint64(count), uint64(len(names))))
	for i, name := range names[offset:end] {
		dirents = append(dirents, p9.Dirent{
			QID:    d.a.qids.Get(p9.TypeRegular),
			Type:   p9.TypeRegular,
			Offset: offset + uint64(i),
			Name:   name,
		})
	}
	return dirents, nil
}

// eventchannel is an append-only file.
type eventchannel struct {
	statfs
	p9.DefaultWalkGetAttr
	templatefs.ReadOnlyFile
	templatefs.NilCloser
	templatefs.NotDirectoryFile
	templatefs.NotSymlinkFile
	templatefs.NoopRenamed
	templatefs.NotLockable

	qid p9.QID
	w   io.Writer

	opened bool
	offset int64
}

var _ p9.File = &eventchannel{}

// Walk implements p9.File.Walk.
func (f *eventchannel) Walk(names []string) ([]p9.QID, p9.File, error) {
	if len(names) == 0 {
		return []p9.QID{f.qid}, f, nil
	}
	return nil, nil, linux.ENOTDIR
}

// Open implements p9.File.Open.
func (f *eventchannel) Open(mode p9.OpenFlags) (p9.QID, uint32, error) {
	if mode == p9.ReadOnly || mode == p9.ReadWrite {
		// Append-only file.
		return p9.QID{}, 0, linux.EINVAL
	}

	f.opened = true
	return f.qid, 4096, nil
}

func (f *eventchannel) FSync() error {
	return nil
}

func (f *eventchannel) WriteAt(p []byte, offset int64) (int, error) {
	if !f.opened {
		return 0, linux.EBADF
	}
	if f.offset != offset {
		return 0, linux.EINVAL
	}
	n, err := f.w.Write(p)
	f.offset = offset + int64(n)
	return n, err
}

// GetAttr implements p9.File.GetAttr.
func (f *eventchannel) GetAttr(req p9.AttrMask) (p9.QID, p9.AttrMask, p9.Attr, error) {
	return f.qid, req, p9.Attr{
		Mode:      p9.ModeRegular | 0777,
		UID:       0,
		GID:       0,
		NLink:     0,
		Size:      uint64(f.Reader.Size()),
		BlockSize: 4096, /* whatever? */
	}, nil
}

package source

import (
	"github.com/mgoltzsche/ctnr/pkg/fs"
)

var _ fs.Source = &sourceSymlink{}

type sourceSymlink struct {
	attrs fs.FileAttrs
}

func NewSourceSymlink(attrs fs.FileAttrs) fs.Source {
	attrs.Mode = 0
	return &sourceSymlink{attrs}
}

func (s *sourceSymlink) Attrs() fs.NodeInfo {
	return fs.NodeInfo{fs.TypeSymlink, s.attrs}
}

func (s *sourceSymlink) DeriveAttrs() (fs.DerivedAttrs, error) {
	return fs.DerivedAttrs{}, nil
}

func (s *sourceSymlink) Write(path, name string, w fs.Writer, written map[fs.Source]string) (err error) {
	if linkDest, ok := written[s]; ok {
		err = w.Link(path, linkDest)
	} else {
		written[s] = path
		err = w.Symlink(path, s.attrs)
	}
	return
}

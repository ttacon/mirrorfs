package mirrorfs

import (
	"errors"
	"io/ioutil"
	"os"
	"path/filepath"
	"syscall"

	"golang.org/x/net/context"

	"bazil.org/fuse"
	"bazil.org/fuse/fs"
)

// EventHandler is our interface for ensuring a receiver can handle events.
type EventHandler interface {
	HandleEvent(event string, data interface{})
}

// Directory is our contract for what it means to be a directory.
type Directory interface {
	fs.Node
	fs.Handle

	EventHandler
}

// DirectoryFactory is our utility for creating new Directories.
type DirectoryFactory interface {
	NewDirectory(path string) Directory
}

// directoryFactory is our internal DirectoryFactory implementation that handles
// internal hooks.
type directoryFactory struct {
	hooks HookHandler
}

// NewDirectoryFactory creates a new DirectoryFactory with the given
// HookHandler.
func NewDirectoryFactory(hh HookHandler) DirectoryFactory {
	return &directoryFactory{
		hooks: hh,
	}
}

// NewDirectory creates a new directory reference for the directory at the
// given path.
func (df *directoryFactory) NewDirectory(path string) Directory {
	return &dir{
		path:        path,
		HookHandler: df.hooks,
	}
}

// dir is our internal directory implementation.
type dir struct {
	HookHandler

	path string
}

// HandleEvent delegates handling of the event and its data to our internal
// HookHandler.
func (d *dir) HandleEvent(event string, data interface{}) {
	d.HookHandler.Handle(event, data)
}

// Attr sets the attributes for the receiving directory on the given `fuse.Attr`.
func (d *dir) Attr(ctx context.Context, a *fuse.Attr) error {
	lgr := loggerWith(map[string]interface{}{
		"Receiver": "dir",
		"Func":     "Attr",
	})
	lgr.Debug("start", []interface{}{
		d,
		ctx,
		a,
	})
	lgrEnd := func(data ...interface{}) {
		lgr.Debug("end", data)
	}

	d.Handle("attr:start", map[string]interface{}{
		"directory": d,
		"context":   ctx,
		"attr":      a,
	})

	fileInfo, err := os.Stat(d.path)
	if err != nil {
		lgrEnd(d, err)
		return syscall.ENOENT
	}

	stat, ok := fileInfo.Sys().(*syscall.Stat_t)
	if !ok {
		lgrEnd(d, "fileInfo sys not of type *syscall.Stat_t")
		return syscall.ENOENT // find a valid error code for this
	}
	a.Inode = stat.Ino
	a.Mode = fileInfo.Mode()

	d.Handle("attr:end", map[string]interface{}{
		"directory": d,
		"context":   ctx,
		"attr":      a,
	})

	lgrEnd(d, ctx, a)
	return nil
}

// Lookup looks up the given file (file, directory, socket, link, etc) from the
// given directory.
func (d *dir) Lookup(ctx context.Context, req *fuse.LookupRequest, resp *fuse.LookupResponse) (fs.Node, error) {
	lgr := loggerWith(map[string]interface{}{
		"Receiver": "dir",
		"Func":     "Lookup",
	})
	lgr.Debug("start", []interface{}{
		d,
		ctx,
		req,
		resp,
	})
	lgrEnd := func(data ...interface{}) {
		lgr.Debug("end", data)
	}

	d.Handle("Lookup:start", map[string]interface{}{
		"directory": d,
		"context":   ctx,
		"req":       req,
		"resp":      resp,
	})

	fullPath := filepath.Join(
		d.path,
		req.Name,
	)

	fileInfo, err := os.Stat(fullPath)
	if err != nil {
		lgrEnd(d, nil, err)
		return nil, syscall.ENOENT
	}

	var node fs.Node
	if fileInfo.IsDir() {
		node = d.DirectoryFactory().NewDirectory(fullPath)
	} else {
		node = d.FileFactory().NewFile(fullPath)
	}

	if err := node.Attr(ctx, &resp.Attr); err != nil {
		lgrEnd(d, nil, err)
		return nil, err
	}

	lgrEnd(d, node, nil)

	d.Handle("Lookup:end", map[string]interface{}{
		"directory": d,
		"node":      node,
		"error":     nil,
	})

	return node, nil
}

// ReadDirAll returns all entries for the current receiving directory.
func (d *dir) ReadDirAll(ctx context.Context) ([]fuse.Dirent, error) {
	lgr := loggerWith(map[string]interface{}{
		"Receiver": "dir",
		"Func":     "ReadDirAll",
	})
	lgr.Debug("start", []interface{}{
		d,
		ctx,
	})
	lgrEnd := func(data ...interface{}) {
		lgr.Debug("end", data)
	}

	d.Handle("ReadDirAll:start", map[string]interface{}{
		"directory": d,
		"context":   ctx,
	})

	entries, err := ioutil.ReadDir(d.path)
	if err != nil {
		d.Handle("ReadDirAll:end", map[string]interface{}{
			"directory": d,
			"entries":   entries,
			"error":     err,
		})

		lgrEnd(d, nil, err)
		return nil, err
	}

	var toReturn = make([]fuse.Dirent, len(entries))

	for i, entry := range entries {
		newEntry := fuse.Dirent{
			Name: entry.Name(),
		}
		toReturn[i] = newEntry

		stat, ok := entry.Sys().(*syscall.Stat_t)
		if ok {
			newEntry.Inode = stat.Ino
		} else {
			lgrEnd(d, "directory entry not of type *syscall.Stat_t")
			return nil, syscall.ENOENT // find a valid error code for this
		}

		// TODO(ttacon): handle all the other types (sockets, etc)
		if entry.IsDir() {
			newEntry.Type = fuse.DT_Dir
		} else {
			newEntry.Type = fuse.DT_File
		}
	}

	d.Handle("ReadDirAll:end", map[string]interface{}{
		"directory": d,
		"entries":   entries,
		"error":     nil,
	})

	lgrEnd(d, toReturn, nil)
	return toReturn, nil
}

// SetAttr sets the given attributes on the entry.
func (d *dir) Setattr(ctx context.Context, req *fuse.SetattrRequest, resp *fuse.SetattrResponse) error {
	lgr := loggerWith(map[string]interface{}{
		"Receiver": "dir",
		"Func":     "Setattr",
	})
	lgr.Debug("start", []interface{}{
		d,
		ctx,
		req,
		resp,
	})
	lgrEnd := func(data ...interface{}) {
		lgr.Debug("end", data)
	}

	d.Handle("Setattr:start", map[string]interface{}{
		"directory": d,
		"context":   ctx,
		"request":   req,
		"response":  resp,
	})

	// We need to:
	//
	//  1. Set all fields from `req` on the underlying `dir`.
	//  2. Set thos in the `resp.Attr`
	//
	// Primary fields that we care about:
	//   - resp.Attr.Mode = d.Entry.Stat.Mode
	//   - resp.Attr.Size = uint64(d.Entry.Stat.Size)
	//   - resp.Attr.Uid = d.Entry.Stat.Uid
	//   - resp.Attr.Gid = d.Entry.Stat.Gid
	d.Handle("Setattr:end", map[string]interface{}{
		"directory": d,
		"context":   ctx,
		"request":   req,
		"response":  resp,
	})

	lgrEnd(d, nil)
	return nil
}

// Create creates the given file in the current directory.
func (d *dir) Create(ctx context.Context, req *fuse.CreateRequest, resp *fuse.CreateResponse) (fs.Node, fs.Handle, error) {
	lgr := loggerWith(map[string]interface{}{
		"Receiver": "dir",
		"Func":     "Create",
	})
	lgr.Debug("start", []interface{}{
		d,
		ctx,
	})
	lgrEnd := func(data ...interface{}) {
		lgr.Debug("end", data)
	}

	d.Handle("Create:start", map[string]interface{}{
		"directory": d,
		"context":   ctx,
		"request":   req,
		"response":  resp,
	})

	fullpath := filepath.Join(
		d.path,
		req.Name,
	)

	f, err := os.OpenFile(
		fullpath,
		int(req.Flags),
		req.Mode,
	)
	if err != nil {
		lgrEnd(d, nil, nil, err)
		return nil, nil, err
	} else if err := f.Close(); err != nil {
		lgrEnd(d, nil, nil, err)
		return nil, nil, err
	}
	d.Handle("Create:end", map[string]interface{}{
		"directory": d,
		"node":      nil,
		"handle":    nil,
		"error":     nil,
	})

	file := d.FileFactory().NewFile(fullpath)

	lgrEnd(d, file, file, nil)

	return file, file, nil
}

func (d *dir) Remove(ctx context.Context, req *fuse.RemoveRequest) error {
	lgr := loggerWith(map[string]interface{}{
		"Receiver": "dir",
		"Func":     "Remove",
	})
	lgr.Debug("start", []interface{}{
		d,
		ctx,
		req,
	})
	lgrEnd := func(data ...interface{}) {
		lgr.Debug("end", data)
	}

	d.Handle("Remove:start", map[string]interface{}{
		"directory": d,
		"context":   ctx,
		"request":   req,
	})

	err := os.Remove(req.Name)

	d.Handle("Remove:end", map[string]interface{}{
		"directory": d,
		"error":     err,
	})
	lgrEnd(d, err)

	return err
}

func (d *dir) Mkdir(ctx context.Context, req *fuse.MkdirRequest) (fs.Node, error) {
	lgr := loggerWith(map[string]interface{}{
		"Receiver": "dir",
		"Func":     "Mkdir",
	})
	lgr.Debug("start", []interface{}{
		d,
		ctx,
		req,
	})
	lgrEnd := func(data ...interface{}) {
		lgr.Debug("end", data)
	}

	d.Handle("Mkdir:start", map[string]interface{}{
		"directory": d,
		"context":   ctx,
		"request":   req,
	})

	fullPath := filepath.Join(
		d.path,
		req.Name,
	)

	if err := os.Mkdir(fullPath, req.Mode); err != nil {
		d.Handle("Mkdir:end", map[string]interface{}{
			"directory": d,
			"node":      nil,
			"error":     nil,
		})

		lgrEnd(d, nil, err)
		return nil, err
	}

	node := d.DirectoryFactory().NewDirectory(fullPath)

	d.Handle("Mkdir:end", map[string]interface{}{
		"directory": d,
		"node":      node,
		"error":     nil,
	})

	lgrEnd(d, node, nil)

	return node, nil
}

func (d *dir) Rename(ctx context.Context, req *fuse.RenameRequest, node fs.Node) error {
	lgr := loggerWith(map[string]interface{}{
		"Receiver": "dir",
		"Func":     "Rename",
	})
	lgr.Debug("start", []interface{}{
		d,
		ctx,
		req,
	})
	lgrEnd := func(data ...interface{}) {
		lgr.Debug("end", data)
	}

	d.Handle("Rename:start", map[string]interface{}{
		"directory": d,
		"context":   ctx,
		"request":   req,
	})

	newDir, ok := node.(*dir)
	if !ok {
		err := errors.New("unexpected node type")
		d.Handle("Rename:end", map[string]interface{}{
			"directory": d,
			"error":     err,
		})
		lgrEnd(d, err)

		return err
	}

	oldPath := filepath.Join(d.path, req.OldName)
	newPath := filepath.Join(newDir.path, req.NewName)

	if err := os.Rename(oldPath, newPath); err != nil {
		d.Handle("Rename:end", map[string]interface{}{
			"directory": d,
			"error":     err,
		})
		lgrEnd(d, err)

		return err
	}

	d.Handle("Rename:end", map[string]interface{}{
		"directory": d,
		"error":     nil,
	})
	lgrEnd(d, nil)

	return nil
}

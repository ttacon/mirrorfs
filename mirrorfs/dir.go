package mirrorfs

import (
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

	f, err := os.OpenFile(
		req.Name,
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

	file := d.FileFactory().NewFile(req.Name)

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

//
// XXX Should we handle Rename ???
// If yes, how do we treat the files - as new or just the old ones?
// Just the old ones is not good. Cannot get from remote as names changed and
// we are not keeping track of remote names separately. We might, if we need this functionality
//
/* COMMENTED OUT FOR NOW!!
func (d *DIR) Rename(ctx context.Context, req *fuse.RenameRequest, newDir fs.Node) error {
	var oldPrefix, newPrefix string
	nd, ok := newDir.(*DIR)
	if !ok {
		log.WithFields(log.Fields{"newDir": newDir}).Error("Rename: New Dir is not a DIR")
		return syscall.EINVAL	// Should we fix fuse.error ???
	}
	if d.Entry.Prefix != "" {
		oldPrefix = d.Entry.Prefix + "/" + d.Entry.Name
	} else {
		oldPrefix = d.Entry.Name
	}
	if nd.Entry.Prefix != "" {
		newPrefix = nd.Entry.Prefix + "/" + nd.Entry.Name
	} else {
		newPrefix = nd.Entry.Name
	}
	log.WithFields(log.Fields{"Dir": d, "newDir": nd, "Request": req,
			  "Old Prefix": oldPrefix, "New Prefix": newPrefix,
		}).Error("Rename request")
	//XXX
	//XXX Do the locking properly, when we do support rename
	d.RData.lock.Lock()
	foundDir := false
	idx := -1
	for i, ent := range d.RData.Meta.Entries {
		if ent.Prefix == oldPrefix && ent.Name == req.OldName {
			if ent.IsDir == false {
				d.RData.Meta.Entries[i].Prefix = newPrefix
				d.RData.Meta.Entries[i].Name = req.NewName
				d.Entry = d.RData.Meta.Entries[i]
				d.RData.lock.Unlock()
				//XXX We are not verifying the New Dir to be part of Meta.Entries now..
				//XXX Is it even possible? I guess not as it will be lookuped up before
				//XXX this is called...
				if err := saveMeta(d.Acc, d.RData); err != nil {
					log.Error("Rename file: cannot save Meta")
					return err
				}
				return nil
			} else {
				foundDir = true
				idx = i
				break
			}
		}
	}
	if !foundDir {
		d.RData.lock.Unlock()
		return syscall.ENOENT
	}
	// Rename a dir
	d.RData.Meta.Entries[idx].Prefix = newPrefix
	d.RData.Meta.Entries[idx].Name = req.NewName
	d.Entry = d.RData.Meta.Entries[idx]
	oldPrefix = oldPrefix + "/" + req.OldName
	newPrefix = newPrefix + "/" + req.NewName
	for i, e2 := range d.RData.Meta.Entries {
		if e2.Prefix == oldPrefix {
			d.RData.Meta.Entries[i].Prefix = newPrefix
		}
	}
	d.RData.lock.Unlock()
	if err := saveMeta(d.Acc, d.RData); err != nil {
		log.Error("Rename dir: cannot save Meta")
		return err
	}
	return nil
}
*/

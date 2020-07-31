package mirrorfs

import (
	"io/ioutil"
	"os"
	"syscall"

	"golang.org/x/net/context"

	"bazil.org/fuse"
	"bazil.org/fuse/fs"
)

// FileFactory is a utility factory for creating new file references with the
// necessary internal bookkeeping data.
type FileFactory interface {
	NewFile(path string) File
}

// fileFactory is our internal FileFactory implementation that maintains a
// reference to a set of hook handlers.
type fileFactory struct {
	hooks HookHandler
}

// NewFileFactory creates a FileFactory with the given HookHandler.
func NewFileFactory(hh HookHandler) FileFactory {
	return &fileFactory{
		hooks: hh,
	}
}

// NewFile returns a reference to the file that exists at the given path.
func (ff *fileFactory) NewFile(path string) File {
	return &file{
		path:        path,
		HookHandler: ff.hooks,
	}
}

// File is our defined interface for what a mirrorfs.File must be.
type File interface {
	fs.Node
	fs.Handle

	EventHandler
}

// file is our internal file reference that can handle events.
type file struct {
	HookHandler

	path string
}

// HandleEvent is passes the event and the given data to our HookHandler for
// processing.
func (f *file) HandleEvent(event string, data interface{}) {
	f.HookHandler.Handle(event, data)
}

// Attr returns sets the attributes for the current file receiver on the given
// `fuse.Attr` reference.
func (f *file) Attr(ctx context.Context, a *fuse.Attr) error {
	lgr := loggerWith(map[string]interface{}{
		"Receiver": "file",
		"Func":     "Attr",
	})
	lgr.Debug("start", []interface{}{
		f,
		ctx,
		a,
	})
	lgrEnd := func(data ...interface{}) {
		lgr.Debug("end", data)
	}

	f.Handle("Attr:start", map[string]interface{}{
		"file":    f,
		"context": ctx,
		"attr":    a,
	})

	fileInfo, err := os.Stat(f.path)
	if err != nil {
		f.Handle("Attr:end", map[string]interface{}{
			"file":  f,
			"error": err,
		})

		lgrEnd(f, err)

		return syscall.ENOENT
	}

	a.Size = uint64(fileInfo.Size())

	stat, ok := fileInfo.Sys().(*syscall.Stat_t)
	if !ok {
		lgrEnd(f, "fileInfo sys not of type *syscall.Stat_t")
		return syscall.ENOENT // find a valid error code for this
	}
	a.Inode = stat.Ino
	a.Mode = fileInfo.Mode()

	f.Handle("Attr:end", map[string]interface{}{
		"file":  f,
		"error": nil,
	})

	lgrEnd(f, nil)
	return nil
}

/*
// Open opens the file for reading.
func (f *File) Open(
	ctx context.Context,
	req *fuse.OpenRequest,
	resp *fuse.OpenResponse,
) (fs.Handle,error) {


	fsHandler, err := os.OpenFile(f.path, int(req.Flags), f.attr.Mode)
	if err != nil {

		return nil, err
	}
	f.handler = fsHandler


	return f, nil
}
*/

// ReadAll reads the contents of the current file receiver.
func (f *file) ReadAll(ctx context.Context) ([]byte, error) {
	lgr := loggerWith(map[string]interface{}{
		"Receiver": "file",
		"Func":     "ReadAll",
	})
	lgr.Debug("start", []interface{}{
		f,
		ctx,
	})
	lgrEnd := func(data ...interface{}) {
		lgr.Debug("end", data)
	}

	f.Handle("ReadAll:start", map[string]interface{}{
		"file":    f,
		"context": ctx,
	})

	data, err := ioutil.ReadFile(f.path)

	f.Handle("ReadAll:end", map[string]interface{}{
		"file":    f,
		"content": data,
		"error":   err,
	})

	lgrEnd(f, data, err)
	return data, err
}

// Write writes the data in the request to the underlying file in an appending
// fashion.
func (f *file) Write(ctx context.Context, req *fuse.WriteRequest, resp *fuse.WriteResponse) error {
	lgr := loggerWith(map[string]interface{}{
		"Receiver": "file",
		"Func":     "Write",
	})
	lgr.Debug("start", []interface{}{
		f,
		ctx,
	})
	lgrEnd := func(data ...interface{}) {
		lgr.Debug("end", data)
	}

	f.Handle("Write:start", map[string]interface{}{
		"file":     f,
		"context":  ctx,
		"request":  req,
		"response": resp,
	})

	fInfo, err := os.Stat(f.path)
	if err != nil {
		lgrEnd(f, err)
		return err
	}

	file, err := os.OpenFile(f.path, int(req.Flags)|os.O_WRONLY, fInfo.Mode())
	if err != nil {
		lgrEnd(f, err)
		return err
	}

	bytesWritten, err := file.WriteAt(req.Data, req.Offset)
	if err != nil {
		lgrEnd(f, err)
		return err
	} else if err := file.Close(); err != nil {
		lgrEnd(f, err)
		return err
	}

	resp.Size = int(req.Offset) + bytesWritten

	f.Handle("Write:end", map[string]interface{}{
		"file":  f,
		"error": err,
	})

	lgrEnd(f, nil)
	return nil

}

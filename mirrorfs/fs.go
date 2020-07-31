package mirrorfs

import (
	"sync"

	"bazil.org/fuse/fs"
)

// FS implements the hello world file system.
type FS interface {
	Root() (fs.Node, error)
	RootPath() string
	WithHook(event string, hook HookFN) FS
}

type HookFN func(data interface{})
type HookHandler map[string][]HookFN

func (hh HookHandler) Register(event string, fn HookFN) {
	hh[event] = append(hh[event], fn)
}

func (hh HookHandler) DirectoryFactory() DirectoryFactory {
	return NewDirectoryFactory(hh)
}

func (hh HookHandler) FileFactory() FileFactory {
	return NewFileFactory(hh)
}

func (hh HookHandler) Handle(event string, data interface{}) {
	lgr := loggerWith(map[string]interface{}{
		"Receiver": "HookHandler",
		"Func":     "Handle",
	})

	lgr.Debug("start")
	globalFns, _ := hh["*"]

	fns, exists := hh[event]
	if (!exists || len(fns) == 0) && len(globalFns) == 0 {
		lgr.Debug("no funcs to run, exiting")
		lgr.Debug("end")
		return
	}

	fns = append(fns, globalFns...)

	var wg sync.WaitGroup
	for _, fn := range fns {
		wg.Add(1)
		go func(data interface{}) {
			fn(data)
			wg.Done()
		}(data)
	}

	wg.Wait()

	lgr.Debug("end")
}

type mirrorFS struct {
	root  string
	hooks HookHandler
}

func NewMirrorFS(root string) FS {
	return &mirrorFS{
		root:  root,
		hooks: HookHandler{},
	}
}

func (m *mirrorFS) Root() (fs.Node, error) {
	return m.hooks.DirectoryFactory().NewDirectory(
		m.root,
	), nil
}

func (m *mirrorFS) RootPath() string {
	return m.root
}

func (m *mirrorFS) WithHook(event string, hook HookFN) FS {
	m.hooks.Register(event, hook)
	return m
}

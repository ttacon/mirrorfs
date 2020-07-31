package main

import (
	"errors"
	"fmt"
	"log"

	"github.com/kr/pretty"
	cli "github.com/urfave/cli/v2"

	"bazil.org/fuse"
	"bazil.org/fuse/fs"

	"github.com/ttacon/mirrorfs/mirrorfs"
)

func mirrorFunc(c *cli.Context) error {
	mountPath := c.String("mount")
	if len(mountPath) == 0 {
		return errors.New("must provide valid mount path")
	}

	conn, err := fuse.Mount(
		c.String("mount"),
		fuse.FSName("scoped"),
		fuse.Subtype("scopefs"),
	)
	if err != nil {
		log.Fatal(err)
	}
	defer conn.Close()

	// cwd, err := os.Getwd()

	mirror := c.String("mirror")
	if len(mirror) == 0 {
		mirror = "/"
	}

	loggingLevel := c.String("logLevel")
	if len(loggingLevel) > 0 {
		mirrorfs.SetLogLevel(loggingLevel)
	}

	mirrFS := mirrorfs.NewMirrorFS(
		mirror,
	).WithHook("*", func(data interface{}) {
		fmt.Println("-----[running hook]-----")
		pretty.Println(data)
	})

	err = fs.Serve(conn, mirrFS)
	if err != nil {
		log.Fatal(err)
	}

	return nil
}

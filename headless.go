//go:build vivahead

// This file links a headless line protocol into a test build. It is behind the
// vivahead build tag, so a release binary never contains it:
//
//	go build -tags vivahead -o mdnote-headless.exe .
//	VIVA_HEADLESS=1 ./mdnote-headless.exe [folder]
//
// The process then serves the protocol documented on headless.Serve on standard
// input and output instead of opening a window. It lays the page out with the
// reference text metrics rather than the platform's own engine, so it needs no
// GPU or font service and runs anywhere.
package main

import (
	"fmt"
	"os"

	"github.com/vivaui/viva/app"
	"github.com/vivaui/viva/gfx"
	"github.com/vivaui/viva/headless"
)

func init() {
	if os.Getenv("VIVA_HEADLESS") != "" {
		app.UseRunner(runHeadless)
	}
}

func runHeadless(config app.Config, canvas *gfx.Canvas) error {
	session, err := headless.Open(config, canvas, headless.Options{ReferenceText: true})
	if err != nil {
		return err
	}
	defer session.Close()
	snapshot := session.Snapshot()
	fmt.Printf("ok ready %dx%d\n", snapshot.Width, snapshot.Height)
	return headless.Serve(session, os.Stdin, os.Stdout)
}

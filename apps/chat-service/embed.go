package main

import (
	"embed"
	"io/fs"
	"os"
)

//go:embed all:frontend
var frontendEmbedFS embed.FS

var frontendFS fs.FS

func init() {
	sub, err := fs.Sub(frontendEmbedFS, "frontend")
	if err != nil {
		frontendFS = nil
		return
	}
	frontendFS = sub
}

func getFrontendFS() fs.FS {
	if frontendFS == nil {
		return nil
	}
	dist, err := fs.Sub(frontendFS, "dist")
	if err != nil {
		return nil
	}
	return dist
}

func hasFrontend() bool {
	if frontendFS == nil {
		return false
	}
	_, err := fs.Stat(frontendFS, "dist")
	if err != nil {
		return false
	}
	entries, err := fs.ReadDir(frontendFS, "dist")
	if err != nil {
		return false
	}
	return len(entries) > 0
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

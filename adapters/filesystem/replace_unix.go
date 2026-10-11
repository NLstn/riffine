//go:build darwin || linux || freebsd || openbsd || netbsd || dragonfly

package filesystem

import "os"

func replaceDocument(staged, path string) error { return os.Rename(staged, path) }
func syncDocumentDirectory(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return f.Sync()
}

//go:build !darwin && !linux && !freebsd && !openbsd && !netbsd && !dragonfly && !windows

package filesystem

import "errors"

func replaceDocument(string, string) error {
	return errors.New("atomic document replacement is unavailable on this host")
}
func syncDocumentDirectory(string) error {
	return errors.New("durable document recovery is unavailable on this host")
}

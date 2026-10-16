// riffine-desktop is the private native Windows UI control host. Stdout is
// reserved for protocol responses; diagnostics go to stderr.
package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"time"

	"github.com/NLstn/riffine/adapters/filesystem"
	"github.com/NLstn/riffine/adapters/native"
	"github.com/NLstn/riffine/core/audio"
	"github.com/NLstn/riffine/core/practice"
	"github.com/NLstn/riffine/internal/desktop"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	if err := run(ctx); err != nil && ctx.Err() == nil {
		fmt.Fprintln(os.Stderr, "riffine-desktop:", err)
		os.Exit(1)
	}
}

func run(ctx context.Context) error {
	service := practice.Service{Resources: filesystem.Host{}, Store: filesystem.Host{}, Decoder: audio.WAVDecoder{}}
	check, cancel := context.WithTimeout(ctx, 10*time.Second)
	if path, err := native.Find(); err == nil {
		adapter := native.Adapter{Executable: path, Stderr: os.Stderr}
		if decoder, err := native.NewDecoder(check, adapter); err == nil {
			service.Decoder = decoder
			if adapter.CheckSourcePreview(check) == nil {
				service.SourcePreviewer = adapter
			}
		} else {
			fmt.Fprintln(os.Stderr, "Native audio unavailable:", err)
		}
	}
	cancel()
	host := desktop.Host{Service: service}
	if root, err := os.UserCacheDir(); err == nil {
		host.Cache = filesystem.PeakCache{Directory: filepath.Join(root, "Riffine", "Waveforms")}
	}
	defer host.Close()
	return desktop.Serve(ctx, os.Stdin, os.Stdout, host.Handle)
}

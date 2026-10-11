package filesystem

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"sync"
	"testing"
)

func TestPeakCachePublicationAndCancellation(t *testing.T) {
	c := PeakCache{Directory: filepath.Join(t.TempDir(), "音楽-cache")}
	key := "opaque/../key"
	a := bytes.Repeat([]byte("a"), 32768)
	b := bytes.Repeat([]byte("b"), 32768)
	ctx := context.Background()
	if err := c.Put(ctx, key, a); err != nil {
		t.Fatal(err)
	}
	got, err := c.Get(ctx, key, len(a))
	if err != nil || !bytes.Equal(got, a) {
		t.Fatal(err)
	}
	cancelCtx, cancel := context.WithCancel(ctx)
	cancel()
	if err = c.Put(cancelCtx, key, b); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	got, err = c.Get(ctx, key, len(a))
	if err != nil || !bytes.Equal(got, a) {
		t.Fatal("cancel replaced cache", err)
	}
	if _, err = c.Get(ctx, key, len(a)-1); err == nil {
		t.Fatal("oversized cache read accepted")
	}
	var writers sync.WaitGroup
	writers.Add(1)
	failure := make(chan error, 1)
	go func() {
		defer writers.Done()
		for n := 0; n < 20; n++ {
			data := a
			if n%2 == 0 {
				data = b
			}
			if e := c.Put(ctx, key, data); e != nil {
				failure <- e
				return
			}
		}
	}()
	for n := 0; n < 100; n++ {
		data, e := c.Get(ctx, key, len(a))
		if e != nil || (!bytes.Equal(data, a) && !bytes.Equal(data, b)) {
			t.Fatal("observed incomplete cache", e)
		}
	}
	writers.Wait()
	select {
	case e := <-failure:
		t.Fatal(e)
	default:
	}
	entries, err := os.ReadDir(c.Directory)
	if err != nil || len(entries) != 1 {
		t.Fatal("staging files leaked", entries, err)
	}
}

func TestPeakCacheReplacementRetainsOpenReader(t *testing.T) {
	ctx := context.Background()
	c := PeakCache{Directory: t.TempDir()}
	if err := c.Put(ctx, "key", []byte("old")); err != nil {
		t.Fatal(err)
	}
	path, _ := c.path("key")
	old, err := openPeakCache(path)
	if err != nil {
		t.Fatal(err)
	}
	defer old.Close()
	if err = c.Put(ctx, "key", []byte("new")); err != nil {
		t.Fatal(err)
	}
	data, err := io.ReadAll(old)
	if err != nil || string(data) != "old" {
		t.Fatal("open reader changed", string(data), err)
	}
	data, err = c.Get(ctx, "key", 3)
	if err != nil || string(data) != "new" {
		t.Fatal("new cache not published", string(data), err)
	}
}

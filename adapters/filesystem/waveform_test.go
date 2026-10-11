package filesystem

import (
	"bytes"
	"context"
	"errors"
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

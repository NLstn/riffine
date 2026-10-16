// Package desktop hosts the shared session bridge over private process pipes.
// There is no listening socket, and audio samples never enter this protocol.
package desktop

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sync"

	"github.com/NLstn/riffine/adapters/binding"
	"github.com/NLstn/riffine/core/practice"
)

const Version = 1
const MaxResponseBytes = 8 << 20

type Request struct {
	Version int             `json:"version"`
	ID      uint64          `json:"id"`
	Cancel  bool            `json:"cancel,omitempty"`
	Request json.RawMessage `json:"request,omitempty"`
}

type Response struct {
	Version  int    `json:"version"`
	ID       uint64 `json:"id"`
	Response any    `json:"response"`
}

type Handler func(context.Context, []byte) any

func decodeEnvelope(line []byte) (Request, error) {
	var q Request
	decoder := json.NewDecoder(bytes.NewReader(line))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&q); err != nil {
		return q, fmt.Errorf("desktop envelope: %w", err)
	}
	var extra any
	if decoder.Decode(&extra) != io.EOF || q.Version != Version || q.ID == 0 ||
		(q.Cancel && len(q.Request) != 0) || (!q.Cancel && (len(q.Request) == 0 || len(q.Request) > binding.MaxRequestBytes)) {
		return q, errors.New("invalid desktop envelope or protocol version")
	}
	return q, nil
}

// Serve owns input and any closable output until EOF or cancellation. Exactly one operation may be in
// flight. A cancel envelope targets that operation's ID and has no reply; the
// operation still returns its final response before another operation starts.
// EOF cancels and joins the worker, including any owned native audio process.
func Serve(ctx context.Context, input io.ReadCloser, output io.Writer, handle Handler) error {
	lifetime, cancel := context.WithCancel(ctx)
	closePipes := func() {
		_ = input.Close()
		// Closing an owned output pipe also interrupts a blocked reply when the
		// parent closes without draining it. Request cancellation keeps it open.
		if closer, ok := output.(io.Closer); ok {
			_ = closer.Close()
		}
	}
	stop := context.AfterFunc(lifetime, closePipes)
	var worker sync.WaitGroup
	readerDone := make(chan struct{})
	defer func() { cancel(); closePipes(); stop(); worker.Wait(); <-readerDone }()
	var mu sync.Mutex
	var active uint64
	var abort context.CancelFunc
	var outputErr error
	var lastID uint64
	scanner := bufio.NewScanner(input)
	scanner.Buffer(make([]byte, 4096), binding.MaxRequestBytes+4096)
	requests := make(chan Request, 1)
	var readErr error
	// Keep EOF detection independent of response publication. In particular,
	// parent shutdown must interrupt a blocked response even while a cancellation
	// message is waiting for the operation's state lock.
	go func() {
		defer close(readerDone)
		defer cancel()
		for scanner.Scan() {
			q, err := decodeEnvelope(scanner.Bytes())
			if err != nil {
				readErr = err
				return
			}
			select {
			case requests <- q:
			case <-lifetime.Done():
				return
			}
		}
		readErr = scanner.Err()
	}()
	for lifetime.Err() == nil {
		var q Request
		select {
		case q = <-requests:
		case <-lifetime.Done():
			continue
		}
		if lifetime.Err() != nil {
			break
		}
		mu.Lock()
		if q.Cancel {
			if active == q.ID && abort != nil {
				abort()
			}
			mu.Unlock()
			continue
		}
		if active != 0 || q.ID <= lastID {
			mu.Unlock()
			return errors.New("desktop operations must be bounded, ordered and serialized")
		}
		lastID, active = q.ID, q.ID
		operation, done := context.WithCancel(lifetime)
		abort = done
		worker.Add(1)
		go func(q Request) {
			defer worker.Done()
			defer done()
			result := handle(operation, q.Request)
			data, err := json.Marshal(Response{Version, q.ID, result})
			if err == nil && len(data) > MaxResponseBytes {
				err = errors.New("desktop response exceeds limit")
			}
			if err != nil {
				data, _ = json.Marshal(Response{Version, q.ID, binding.Response{Version: binding.Version,
					Error: &practice.Error{Version: practice.APIVersion, Code: practice.HostFailure, Message: err.Error()}}})
			}
			// Publish the reply and release the operation atomically with respect to
			// the input loop, so the client's next request cannot race completion.
			mu.Lock()
			if lifetime.Err() == nil {
				_, err := output.Write(append(data, '\n'))
				if err != nil && lifetime.Err() == nil {
					outputErr = err
					cancel()
				}
			}
			active, abort = 0, nil
			mu.Unlock()
		}(q)
		mu.Unlock()
	}
	cancel()
	worker.Wait()
	<-readerDone
	if outputErr != nil {
		return outputErr
	}
	if ctx.Err() != nil {
		return ctx.Err()
	}
	return readErr
}

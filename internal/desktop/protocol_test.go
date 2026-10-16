package desktop

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"strings"
	"testing"
	"time"
)

func TestProtocolCancellationAndNextOperation(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	input, send := io.Pipe()
	output, receive := io.Pipe()
	defer output.Close()
	entered := make(chan struct{})
	finished := make(chan error, 1)
	go func() {
		finished <- Serve(ctx, input, receive, func(ctx context.Context, data []byte) any {
			if string(data) == `{"type":"wait"}` {
				close(entered)
				<-ctx.Done()
				return map[string]string{"error": "cancelled"}
			}
			return map[string]string{"result": "next"}
		})
		receive.Close()
	}()
	write := func(line string) {
		t.Helper()
		if _, err := io.WriteString(send, line+"\n"); err != nil {
			t.Fatal(err)
		}
	}
	decoder := json.NewDecoder(output)
	write(`{"version":1,"id":1,"request":{"type":"wait"}}`)
	select {
	case <-entered:
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	write(`{"version":1,"id":1,"cancel":true}`)
	var reply struct {
		ID       uint64            `json:"id"`
		Response map[string]string `json:"response"`
	}
	if err := decoder.Decode(&reply); err != nil || reply.ID != 1 || reply.Response["error"] != "cancelled" {
		t.Fatalf("cancelled reply: %+v, %v", reply, err)
	}
	// A late cancellation must not cancel the next operation or add a reply.
	write(`{"version":1,"id":1,"cancel":true}`)
	write(`{"version":1,"id":2,"request":{"type":"next"}}`)
	if err := decoder.Decode(&reply); err != nil || reply.ID != 2 || reply.Response["result"] != "next" {
		t.Fatalf("next reply: %+v, %v", reply, err)
	}
	send.Close()
	if err := <-finished; err != nil {
		t.Fatal(err)
	}
}

func TestProtocolEOFJoinsCancelledWorker(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	input, send := io.Pipe()
	entered, joined := make(chan struct{}), make(chan struct{})
	finished := make(chan error, 1)
	go func() {
		finished <- Serve(ctx, input, io.Discard, func(ctx context.Context, _ []byte) any {
			close(entered)
			<-ctx.Done()
			defer close(joined)
			return map[string]bool{"cancelled": true}
		})
	}()
	io.WriteString(send, "{\"version\":1,\"id\":1,\"request\":{}}\n")
	select {
	case <-entered:
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	send.Close()
	if err := <-finished; err != nil {
		t.Fatal(err)
	}
	select {
	case <-joined:
	default:
		t.Fatal("Serve returned before its worker released owned resources")
	}
}

func TestProtocolRejectsMalformedAndOversizedEnvelopes(t *testing.T) {
	for _, line := range []string{
		`{"version":2,"id":1,"request":{}}`,
		`{"version":1,"id":0,"request":{}}`,
		`{"version":1,"id":1,"request":{},"extra":true}`,
		`{"version":1,"id":1,"cancel":true,"request":{}}`,
		`{"version":1,"id":1,"request":{}} {}`,
		strings.Repeat("x", 2<<20),
	} {
		var out bytes.Buffer
		if err := Serve(context.Background(), io.NopCloser(strings.NewReader(line+"\n")), &out,
			func(context.Context, []byte) any { t.Error("invalid envelope reached handler"); return nil }); err == nil {
			t.Error("accepted malformed desktop envelope")
		}
	}
}

func TestEOFInterruptsBlockedReply(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	input, send := io.Pipe()
	output, receive := io.Pipe()
	defer output.Close()
	writing := make(chan struct{})
	finished := make(chan error, 1)
	go func() {
		finished <- Serve(ctx, input, &signalledWriter{receive, writing}, func(context.Context, []byte) any {
			return strings.Repeat("x", 1<<20)
		})
	}()
	io.WriteString(send, "{\"version\":1,\"id\":1,\"request\":{}}\n")
	select {
	case <-writing:
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	// Deliberately never drain output, as when a GUI closes mid-response.
	// Cancellation must not prevent the independent reader from noticing EOF.
	io.WriteString(send, "{\"version\":1,\"id\":1,\"cancel\":true}\n")
	send.Close()
	select {
	case err := <-finished:
		if err != nil {
			t.Fatal(err)
		}
	case <-ctx.Done():
		t.Fatal("EOF did not interrupt the blocked reply")
	}
}

type signalledWriter struct {
	io.WriteCloser
	entered chan struct{}
}

func (w *signalledWriter) Write(data []byte) (int, error) {
	close(w.entered)
	return w.WriteCloser.Write(data)
}

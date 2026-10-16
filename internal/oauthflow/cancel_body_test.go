package oauthflow

import (
	"context"
	"errors"
	"io"
	"testing"
	"time"
)

type cancelBodyReader func([]byte) (int, error)

func (f cancelBodyReader) Read(p []byte) (int, error) { return f(p) }

func TestCancelBodyPreservesRequestError(t *testing.T) {
	for _, duringRead := range []bool{false, true} {
		t.Run(map[bool]string{false: "before read", true: "during read"}[duringRead], func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			reads := 0
			body := &cancelBody{ctx: ctx, cancel: cancel, ReadCloser: io.NopCloser(cancelBodyReader(func(p []byte) (int, error) {
				reads++
				cancel()
				p[0] = 'x'
				return 1, io.EOF
			}))}
			if !duringRead {
				cancel()
			}
			buffer := make([]byte, 1)
			n, err := body.Read(buffer)
			if !errors.Is(err, context.Canceled) {
				t.Fatalf("lost caller cancellation: %v", err)
			}
			if duringRead && (reads != 1 || n != 1 || buffer[0] != 'x') {
				t.Fatalf("lost bytes returned with cancellation: reads=%d n=%d data=%q", reads, n, buffer)
			}
			if !duringRead && (reads != 0 || n != 0) {
				t.Fatalf("read body after cancellation: reads=%d n=%d", reads, n)
			}
		})
	}
	ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
	defer cancel()
	body := &cancelBody{ctx: ctx, cancel: cancel, ReadCloser: io.NopCloser(cancelBodyReader(func([]byte) (int, error) {
		t.Fatal("read body after deadline")
		return 0, io.EOF
	}))}
	if _, err := body.Read(make([]byte, 1)); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("lost request deadline: %v", err)
	}
}

func TestCancelBodyPreservesLiveResponse(t *testing.T) {
	readError := errors.New("response read failed")
	for _, want := range []error{nil, io.EOF, readError} {
		ctx, cancel := context.WithCancel(context.Background())
		body := &cancelBody{ctx: ctx, cancel: cancel, ReadCloser: io.NopCloser(cancelBodyReader(func(p []byte) (int, error) {
			p[0] = 'x'
			return 1, want
		}))}
		buffer := make([]byte, 1)
		n, err := body.Read(buffer)
		if n != 1 || buffer[0] != 'x' || err != want || ctx.Err() != nil {
			t.Fatalf("changed live response: n=%d data=%q err=%v want=%v ctx=%v", n, buffer, err, want, ctx.Err())
		}
		if err := body.Close(); err != nil || !errors.Is(ctx.Err(), context.Canceled) {
			t.Fatalf("close did not release request context: err=%v ctx=%v", err, ctx.Err())
		}
	}
}

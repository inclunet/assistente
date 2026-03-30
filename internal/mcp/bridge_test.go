package mcp

import (
	"fmt"
	"io"
	"net"
	"testing"
)

func TestIsSessionOrTransportError(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{"nil", nil, false},
		{"generic error", fmt.Errorf("something went wrong"), false},
		{"auth error", fmt.Errorf("401 unauthorized"), false},

		// io errors
		{"io.EOF", io.EOF, true},
		{"io.UnexpectedEOF", io.ErrUnexpectedEOF, true},
		{"wrapped EOF", fmt.Errorf("read: %w", io.EOF), true},

		// SessionExpiredError
		{"session expired 404", &SessionExpiredError{StatusCode: 404}, true},
		{"session expired 410", &SessionExpiredError{StatusCode: 410}, true},
		{"wrapped session expired", fmt.Errorf("call failed: %w", &SessionExpiredError{StatusCode: 404}), true},

		// string-based detection
		{"connection reset", fmt.Errorf("read tcp: connection reset by peer"), true},
		{"connection refused", fmt.Errorf("dial tcp: connection refused"), true},
		{"broken pipe", fmt.Errorf("write: broken pipe"), true},
		{"session expired text", fmt.Errorf("session expired"), true},
		{"session not found", fmt.Errorf("session not found"), true},
		{"closed connection", fmt.Errorf("use of closed network connection"), true},
		{"eof in message", fmt.Errorf("unexpected eof reading response"), true},

		// SDK standalone SSE stream errors
		{"connection closed", fmt.Errorf("connection closed: calling tools/call"), true},
		{"client is closing", fmt.Errorf("client is closing: standalone SSE stream"), true},
		{"exceeded retries", fmt.Errorf("standalone SSE stream: exceeded 5 retries without progress"), true},
		{"full SSE error", fmt.Errorf(`connection closed: calling "tools/call": client is closing: standalone SSE stream: exceeded 5 retries without progress`), true},

		// net.OpError
		{"net.OpError", &net.OpError{Op: "read", Err: fmt.Errorf("reset")}, true},
		{"wrapped net.OpError", fmt.Errorf("call: %w", &net.OpError{Op: "dial", Err: fmt.Errorf("refused")}), true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := isSessionOrTransportError(tc.err)
			if got != tc.want {
				t.Errorf("isSessionOrTransportError(%v): got %v, want %v", tc.err, got, tc.want)
			}
		})
	}
}

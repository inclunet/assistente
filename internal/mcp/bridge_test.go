package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"strings"
	"testing"
	"time"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestSessionTerminationClassificationForToolErrors(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{"nil", nil, false},
		{"deadline", context.DeadlineExceeded, false},
		{"wrapped deadline", fmt.Errorf("call: %w", context.DeadlineExceeded), false},
		{"cancelled", context.Canceled, false},
		{"business exceeded", fmt.Errorf("quota exceeded"), false},
		{"generic error", fmt.Errorf("something went wrong"), false},
		{"auth error", fmt.Errorf("401 unauthorized"), false},

		{"SDK closed", mcpsdk.ErrConnectionClosed, true},
		{"SDK closed with SSE cause", fmt.Errorf("%w: exceeded 5 retries", mcpsdk.ErrConnectionClosed), true},
		{"closed network", net.ErrClosed, true},
		{"network deadline", &net.OpError{Op: "read", Err: context.DeadlineExceeded}, false},

		// io errors
		{"io.EOF", io.EOF, true},
		{"io.UnexpectedEOF", io.ErrUnexpectedEOF, true},
		{"wrapped EOF", fmt.Errorf("read: %w", io.EOF), true},

		// SessionExpiredError
		{"session expired 404", &SessionExpiredError{StatusCode: 404}, true},
		{"session expired 410", &SessionExpiredError{StatusCode: 410}, true},
		{"wrapped session expired", fmt.Errorf("call failed: %w", &SessionExpiredError{StatusCode: 404}), true},

		// Texto remoto não comprova encerramento: o SDK usa sentinela e Wait.
		{"connection reset", fmt.Errorf("read tcp: connection reset by peer"), false},
		{"connection refused", fmt.Errorf("dial tcp: connection refused"), false},
		{"broken pipe", fmt.Errorf("write: broken pipe"), false},
		{"session expired text", fmt.Errorf("session expired"), false},
		{"session not found", fmt.Errorf("session not found"), false},
		{"closed connection", fmt.Errorf("use of closed network connection"), false},
		{"eof in message", fmt.Errorf("unexpected eof reading response"), false},

		// Causas textuais do SSE só são terminais quando reportadas por Wait
		// ou acompanhadas da sentinela tipada do SDK.
		{"connection closed", fmt.Errorf("connection closed: calling tools/call"), false},
		{"client is closing", fmt.Errorf("client is closing: standalone SSE stream"), false},
		{"exceeded retries", fmt.Errorf("standalone SSE stream: exceeded 5 retries without progress"), false},
		{"full SSE error", fmt.Errorf(`connection closed: calling "tools/call": client is closing: standalone SSE stream: exceeded 5 retries without progress`), false},

		// net.OpError
		{"net.OpError", &net.OpError{Op: "read", Err: fmt.Errorf("reset")}, true},
		{"wrapped net.OpError", fmt.Errorf("call: %w", &net.OpError{Op: "dial", Err: fmt.Errorf("refused")}), true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := isSessionTerminationError(tc.err)
			if got != tc.want {
				t.Errorf("isSessionTerminationError(%v): got %v, want %v", tc.err, got, tc.want)
			}
		})
	}
}

// Usa uma sessão real do SDK: um erro de uma chamada não pode invalidar
// as demais ferramentas nem solicitar a reconexão do servidor.
func TestBridgeCallFailurePreservesSession(t *testing.T) {
	for _, mode := range []string{"deadline", "cancel", "remote-error"} {
		t.Run(mode, func(t *testing.T) {
			server := mcpsdk.NewServer(&mcpsdk.Implementation{Name: "call-isolation", Version: "1"}, nil)
			entered := make(chan struct{})
			slow := &mcpsdk.Tool{Name: "slow", InputSchema: map[string]any{"type": "object"}}
			server.AddTool(slow, func(ctx context.Context, _ *mcpsdk.CallToolRequest) (*mcpsdk.CallToolResult, error) {
				close(entered)
				if mode == "remote-error" {
					return nil, fmt.Errorf("quota exceeded")
				}
				<-ctx.Done()
				return nil, ctx.Err()
			})
			fast := &mcpsdk.Tool{Name: "fast", InputSchema: map[string]any{"type": "object"}}
			server.AddTool(fast, func(context.Context, *mcpsdk.CallToolRequest) (*mcpsdk.CallToolResult, error) {
				return &mcpsdk.CallToolResult{Content: []mcpsdk.Content{&mcpsdk.TextContent{Text: "ok"}}}, nil
			})
			clientTransport, serverTransport := mcpsdk.NewInMemoryTransports()
			serverSession, err := server.Connect(context.Background(), serverTransport, nil)
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = serverSession.Close() }()
			client := mcpsdk.NewClient(&mcpsdk.Implementation{Name: "test", Version: "1"}, nil)
			session, err := client.Connect(context.Background(), clientTransport, nil)
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = session.Close() }()
			failures := make(chan error, 2)
			bridge := NewMCPToolBridge("test", slow, session)
			bridge.onSessionError = func(_ string, err error) { failures <- err }
			callCtx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			if mode == "cancel" {
				go func() {
					select {
					case <-entered:
						cancel()
					case <-callCtx.Done():
					}
				}()
			}
			result, err := bridge.Execute(callCtx, json.RawMessage(`{}`))
			if err != nil || !result.IsError {
				t.Fatalf("falha da chamada não preservada: %+v %v", result, err)
			}
			select {
			case <-entered:
			default:
				t.Fatal("chamada não chegou ao servidor")
			}
			expected := "deadline exceeded"
			if mode == "cancel" {
				expected = "canceled"
			}
			if mode == "remote-error" {
				expected = "quota exceeded"
			}
			if !strings.Contains(result.Content, expected) {
				t.Fatalf("causa perdida: %s", result.Content)
			}
			nextCtx, nextCancel := context.WithTimeout(context.Background(), time.Second)
			defer nextCancel()
			result, err = NewMCPToolBridge("test", fast, session).Execute(nextCtx, json.RawMessage(`{}`))
			if err != nil || result.IsError || result.Content != "ok" {
				t.Fatalf("outra tool indisponível: %+v %v", result, err)
			}
			select {
			case err := <-failures:
				t.Fatalf("erro de chamada solicitou reconexão: %v", err)
			case <-time.After(100 * time.Millisecond):
			}
			// O mesmo bridge ainda deve sinalizar um encerramento real da sessão.
			if err := session.Close(); err != nil {
				t.Fatal(err)
			}
			result, err = bridge.Execute(nextCtx, json.RawMessage(`{}`))
			if err != nil || !result.IsError {
				t.Fatalf("sessão fechada aceita: %+v %v", result, err)
			}
			select {
			case err := <-failures:
				if !errors.Is(err, mcpsdk.ErrConnectionClosed) {
					t.Fatalf("causa de encerramento perdida: %v", err)
				}
			case <-time.After(time.Second):
				t.Fatal("sessão encerrada não solicitou recuperação")
			}
		})
	}
}

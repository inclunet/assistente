package oauthintegrations

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"strings"

	"assistente/internal/oauthflow"
)

// MCPHTTPClient applies protocol extensions without replacing the caller's
// network consent, redirect policy, timeout or credential lifecycle.
func MCPHTTPClient(client *http.Client, resource string) *http.Client {
	if resource != "https://mcp.slack.com/mcp" {
		return client
	}
	copy := *client
	base := client.Transport
	if base == nil {
		base = http.DefaultTransport
	}
	copy.Transport = slackTokenTransport{base: base}
	return &copy
}

type slackTokenTransport struct{ base http.RoundTripper }

func (t slackTokenTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	response, err := t.base.RoundTrip(req)
	if err != nil || req.Method != http.MethodPost || req.URL.String() != "https://slack.com/api/oauth.v2.user.access" || response.StatusCode != http.StatusOK {
		return response, err
	}
	defer func() { _ = response.Body.Close() }()
	raw, err := io.ReadAll(io.LimitReader(response.Body, 1024*1024+1))
	if err != nil || len(raw) > 1024*1024 {
		return nil, oauthflow.ErrReauthorize
	}
	var payload map[string]json.RawMessage
	if json.Unmarshal(raw, &payload) != nil || payload == nil {
		return nil, oauthflow.ErrReauthorize
	}
	// Slack reports some failures with HTTP 200. Never turn those into grants,
	// even if an inconsistent response also contains token fields.
	var ok bool
	if json.Unmarshal(payload["ok"], &ok) != nil || !ok {
		var code string
		_ = json.Unmarshal(payload["error"], &code)
		switch code {
		case "invalid_client_id", "bad_client_secret":
			code = "invalid_client"
		case "invalid_refresh_token", "invalid_grant", "invalid_scope", "invalid_client":
		default:
			code = "invalid_response"
		}
		raw, _ = json.Marshal(map[string]string{"error": code})
		response.StatusCode, response.Status = http.StatusBadRequest, "400 Bad Request"
	} else {
		var kind string
		if json.Unmarshal(payload["token_type"], &kind) != nil || (kind != "user" && !strings.EqualFold(kind, "Bearer")) {
			return nil, oauthflow.ErrReauthorize
		}
		// 'user' describes Slack's token role; the MCP wire scheme is Bearer.
		payload["token_type"] = json.RawMessage(`"Bearer"`)
		if _, present := payload["scope"]; !present {
			var user map[string]json.RawMessage
			if nested, present := payload["authed_user"]; present {
				if json.Unmarshal(nested, &user) != nil {
					return nil, oauthflow.ErrReauthorize
				}
				if scope, present := user["scope"]; present {
					payload["scope"] = scope
				}
			}
		}
		if scope, present := payload["scope"]; present {
			var value string
			if string(scope) == "null" || json.Unmarshal(scope, &value) != nil {
				return nil, oauthflow.ErrReauthorize
			}
			value = strings.Join(strings.FieldsFunc(value, func(r rune) bool { return r == ',' || r == ' ' }), " ")
			payload["scope"], _ = json.Marshal(value)
		}
		raw, err = json.Marshal(payload)
		if err != nil {
			return nil, oauthflow.ErrReauthorize
		}
	}
	// Return a copy so the defer still closes the original response body.
	return normalizedSlackResponse(response, raw), nil
}

func normalizedSlackResponse(response *http.Response, raw []byte) *http.Response {
	copy := *response
	copy.Header = response.Header.Clone()
	copy.Header.Set("Content-Type", "application/json")
	copy.Header.Del("Content-Length")
	copy.Body = io.NopCloser(bytes.NewReader(raw))
	copy.ContentLength = int64(len(raw))
	return &copy
}

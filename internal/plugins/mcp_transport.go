package plugins

import (
	"encoding/json"
	"golang.org/x/net/http/httpguts"
	"io/fs"
	"net"
	"net/url"
	"path"
	"strings"
)

// mcpTransport validates the transport-specific 1.0.0 shape before offering
// a component. A valid declaration may still require an unavailable runtime.
func mcpTransport(raw json.RawMessage) (string, bool) {
	var fields map[string]any
	if json.Unmarshal(raw, &fields) != nil {
		return "", false
	}
	transport, _ := fields["type"].(string)
	switch transport {
	case "stdio":
		if command, _ := fields["command"].(string); !validExecutableToken(command) {
			return "", false
		}
	case "sse", "streamable-http":
		if endpoint, _ := fields["url"].(string); !validMCPEndpoint(endpoint) {
			return "", false
		}
	default:
		return "", false
	}
	for key, value := range fields {
		switch key {
		case "type":
			continue
		case "url":
			if transport == "stdio" {
				return "", false
			}
		case "command":
			if transport != "stdio" {
				return "", false
			}
		case "args":
			args, ok := value.([]any)
			if transport != "stdio" || !ok {
				return "", false
			}
			for _, arg := range args {
				if _, ok := arg.(string); !ok {
					return "", false
				}
			}
		case "env", "headers":
			values, ok := value.(map[string]any)
			if !ok || (key == "env") != (transport == "stdio") {
				return "", false
			}
			seen := map[string]bool{}
			for name, v := range values {
				if key == "headers" {
					lower := strings.ToLower(name)
					headerValue, ok := v.(string)
					if !ok || !httpguts.ValidHeaderFieldName(name) || !httpguts.ValidHeaderFieldValue(headerValue) || seen[lower] {
						return "", false
					}
					seen[lower] = true
				}
				if _, ok := v.(string); !ok || key == "env" && (name == "PLUGIN_ROOT" || name == "PLUGIN_DATA") {
					return "", false
				}
			}
		case "cwd":
			dir, ok := value.(string)
			if transport != "stdio" || !ok || !validWorkingDirectory(dir) {
				return "", false
			}
		default:
			return "", false
		}
	}
	return transport, true
}

func validMCPEndpoint(address string) bool {
	u, err := url.Parse(address)
	if err != nil || u.Hostname() == "" || u.User != nil || strings.Contains(address, "#") {
		return false
	}
	return u.Scheme == "https" || u.Scheme == "http" && (strings.EqualFold(u.Hostname(), "localhost") || net.ParseIP(u.Hostname()).IsLoopback())
}

func validExecutableToken(token string) bool {
	if token == "" || strings.ContainsAny(token, "\\:\x00\r\n") {
		return false
	}
	if path, ok := strings.CutPrefix(token, "./"); ok {
		return path != "." && fs.ValidPath(path)
	}
	return token != "." && token != ".." && !strings.ContainsAny(token, "/ \t")
}

func validWorkingDirectory(dir string) bool {
	for _, base := range []string{".", "${PLUGIN_ROOT}", "${PLUGIN_DATA}"} {
		if dir == base && base != "." {
			return true
		}
		rest, ok := strings.CutPrefix(dir, base+"/")
		if ok {
			clean := path.Clean(rest)
			return !strings.ContainsAny(rest, "\\:\x00") && !path.IsAbs(rest) && clean != ".." && !strings.HasPrefix(clean, "../")
		}
	}
	return false
}

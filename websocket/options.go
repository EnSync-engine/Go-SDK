package websocket

import (
	"fmt"
	"net/url"
	"strings"
)

func parseWebSocketURL(endpoint string) (string, error) {
	if !strings.Contains(endpoint, "://") {
		return "", fmt.Errorf("invalid endpoint format")
	}

	u, err := url.Parse(endpoint)
	if err != nil {
		return "", err
	}

	switch strings.ToLower(u.Scheme) {
	case "ws", "wss":
		// Already correct WebSocket schemes
	case "http":
		u.Scheme = "ws"
	case "https":
		u.Scheme = "wss"
	default:
		return "", fmt.Errorf("unsupported scheme: %s", u.Scheme)
	}

	// Add message endpoint
	u.Path = "/message"
	return u.String(), nil
}

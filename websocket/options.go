package websocket

import (
	"fmt"
	"net/url"
	"strings"
)

const (
	schemeWS    = "ws"
	schemeWSS   = "wss"
	schemeHTTP  = "http"
	schemeHTTPS = "https"
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
	case schemeWS, schemeWSS:
	case schemeHTTP:
		u.Scheme = schemeWS
	case schemeHTTPS:
		u.Scheme = schemeWSS
	default:
		return "", fmt.Errorf("unsupported scheme: %s", u.Scheme)
	}

	// Add message endpoint
	u.Path = "/message"
	return u.String(), nil
}

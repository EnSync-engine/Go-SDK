package grpc

import (
	"fmt"
	"net/url"
	"strings"
)

type Config struct {
	URL       string
	AccessKey string

	MaxRecvMsgSize int
	MaxSendMsgSize int
	TLSConfig      *TLSConfig
}

type TLSConfig struct {
	CertFile           string
	KeyFile            string
	CAFile             string
	ServerName         string
	InsecureSkipVerify bool
}

func parseGRPCURL(endpoint string) (host string, secure bool, err error) {
	if !strings.Contains(endpoint, "://") {
		return "", false, fmt.Errorf("invalid endpoint format, expected scheme://host:port")
	}

	u, err := url.Parse(endpoint)
	if err != nil {
		return "", false, fmt.Errorf("invalid URL format: %w", err)
	}

	switch strings.ToLower(u.Scheme) {
	case "grpc":
		return u.Host, false, nil
	case "grpcs":
		return u.Host, true, nil
	default:
		return "", false, fmt.Errorf("unsupported scheme: %s", u.Scheme)
	}
}

func extractServerName(host string) string {
	if colonIndex := strings.LastIndex(host, ":"); colonIndex != -1 {
		return host[:colonIndex]
	}
	return host
}

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

func parseGRPCURL(endpoint string) (string, error) {
	if !strings.Contains(endpoint, "://") {
		return "", fmt.Errorf("invalid endpoint format, expected scheme://host:port")
	}

	u, err := url.Parse(endpoint)
	if err != nil {
		return "", fmt.Errorf("invalid URL format: %w", err)
	}

	switch strings.ToLower(u.Scheme) {
	case "grpc":
		return u.Host, nil
	case "grpcs":
		return u.Host, nil
	default:
		return "", fmt.Errorf("unsupported scheme: %s", u.Scheme)
	}
}

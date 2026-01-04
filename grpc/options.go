package grpc

import (
	"crypto/tls"
	"fmt"
	"net/url"
	"strings"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
)

const (
	defaultMaxMessageSize = 1024 * 1024
	defaultMaxRecvSize    = 2 * 1024 * 1024
	maxSafeMessageSize    = 10 * 1024 * 1024
)

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

func createClientConn(host string, secure bool) (*grpc.ClientConn, error) {
	creds := getTransportCredentials(host, secure)
	opts := getDialOptions(creds)

	return grpc.NewClient(host, opts...)
}

func getTransportCredentials(host string, secure bool) credentials.TransportCredentials {
	if secure {
		serverName := extractServerName(host)
		return credentials.NewTLS(&tls.Config{
			ServerName: serverName,
			MinVersion: tls.VersionTLS12,
		})
	}
	return insecure.NewCredentials()
}

func getDialOptions(creds credentials.TransportCredentials) []grpc.DialOption {
	return []grpc.DialOption{
		grpc.WithTransportCredentials(creds),
		grpc.WithDefaultCallOptions(
			grpc.MaxCallRecvMsgSize(defaultMaxRecvSize),
			grpc.MaxCallSendMsgSize(defaultMaxMessageSize),
		),
	}
}

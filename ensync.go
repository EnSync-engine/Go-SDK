package ensync

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"regexp"
	"strings"
	"time"

	"golang.org/x/net/http2"
)

// EnSyncConfig holds configuration for the EnSync engine
type EnSyncConfig struct {
	Version   string
	Client    string
	AccessKey string
	IsSecure  bool
	EnsyncURL string
}

// EnSyncInternalAction holds internal action states
type EnSyncInternalAction struct {
	PausePulling []string
	StopPulling  []string
	EndSession   bool
}

// Option represents a functional option for configuring EnSyncEngine
type Option func(*EnSyncEngine)

// WithAccessKey sets the access key for the EnSync engine
func WithAccessKey(key string) Option {
	return func(e *EnSyncEngine) {
		e.config.AccessKey = key
	}
}

// WithVersion sets the API version for the EnSync engine
func WithVersion(version string) Option {
	return func(e *EnSyncEngine) {
		e.config.Version = version
	}
}

// WithTLSConfig sets the TLS configuration for the EnSync engine
func WithTLSConfig(disableTLS bool) Option {
	return func(e *EnSyncEngine) {
		e.tlsConfig = &tls.Config{InsecureSkipVerify: disableTLS}
	}
}

// WithRenewAt sets the renewal time for the EnSync engine
func WithRenewAt(renewAt int64) Option {
	return func(e *EnSyncEngine) {
		e.renewAt = renewAt
	}
}

// WithHTTP2 enables HTTP/2 support for the EnSync engine
func WithHTTP2() Option {
	return func(e *EnSyncEngine) {
		e.useHTTP2 = true
	}
}

// EnSyncRecord represents a record in the system
type EnSyncRecord struct {
	ID      string `json:"id"`
	Block   string `json:"block"`
	Payload any    `json:"payload"`
}

// EnSyncEventPayload represents an event payload
type EnSyncEventPayload struct {
	Data      any            `json:"data,omitempty"`
	Timestamp int64          `json:"timestamp,omitempty"`
	Header    map[string]any `json:"header,omitempty"`
}

// EnSyncPublishOptions represents options for publishing
type EnSyncPublishOptions struct {
	Persist bool
	Headers map[string]any
}

// EnSyncSubscribeOptions represents options for subscribing
type EnSyncSubscribeOptions struct {
	AutoAck bool
}

// EnSyncClient represents a client instance
type EnSyncClient struct {
	engine    *EnSyncEngine
	clientId  string
	accessKey string
}

// EnSyncSubscription represents a subscription to events
type EnSyncSubscription struct {
	client  *EnSyncClient
	event   string
	context context.Context
	cancel  context.CancelFunc
}

// EnSyncEngine represents the main engine
type EnSyncEngine struct {
	config         *EnSyncConfig
	internalAction *EnSyncInternalAction
	client         *http.Client
	tlsConfig      *tls.Config
	useHTTP2       bool
	renewAt        int64
}

// NewEnSyncEngine creates a new instance of EnSyncEngine
func NewEnSyncEngine(ensyncURL string, options ...Option) (*EnSyncEngine, error) {
	config := &EnSyncConfig{
		Version:   "v1",
		IsSecure:  strings.HasPrefix(ensyncURL, "https"),
		EnsyncURL: ensyncURL,
	}

	engine := &EnSyncEngine{
		config: config,
		internalAction: &EnSyncInternalAction{
			PausePulling: make([]string, 0),
			StopPulling:  make([]string, 0),
			EndSession:   false,
		},
	}

	// Apply all options
	for _, opt := range options {
		opt(engine)
	}

	// Update client path based on version
	engine.config.Client = fmt.Sprintf("/http/%s/client", engine.config.Version)

	// Setup HTTP client based on configuration
	if engine.useHTTP2 {
		transport := &http2.Transport{
			TLSClientConfig: engine.tlsConfig,
		}

		engine.client = &http.Client{
			Transport: transport,
		}

	} else {
		// Setup HTTP/1.1 client
		transport := &http.Transport{
			TLSClientConfig: engine.tlsConfig,
		}
		engine.client = &http.Client{Transport: transport}
	}

	return engine, nil
}

// CreateClient creates a new client with the given access key
func (e *EnSyncEngine) CreateClient(ctx context.Context) (*EnSyncClient, error) {
	command := fmt.Sprintf("CONN;ACCESS_KEY=%s", e.config.AccessKey)
	response, err := e.createRequest(ctx, command, nil)
	if err != nil {
		return nil, err
	}

	// Parse response to get client ID
	result, err := e.convertKeyValueToObject(response, "{", "}")
	if err != nil {
		return nil, err
	}

	clientID := result["clientId"]
	if clientID == "" {
		return nil, NewEnSyncError("Failed to create client: no client ID returned", "")
	}

	return &EnSyncClient{
		engine:    e,
		clientId:  clientID,
		accessKey: e.config.AccessKey,
	}, nil
}

func (e *EnSyncEngine) convertKeyValueToObject(data string, startsWith string, endsWith string) (map[string]string, error) {
	if len(data) == 0 {
		return nil, errors.New("no data found")
	}

	convertedRecords := make(map[string]string)

	startIdx := strings.Index(data, startsWith)
	if startIdx == -1 {
		return nil, errors.New("invalid format: missing expected starting character")
	}
	data = data[startIdx:]

	if strings.HasPrefix(data, startsWith) && strings.HasSuffix(data, endsWith) {
		data = data[1 : len(data)-1]
	}

	items := strings.Split(data, ",")
	for _, item := range items {
		parts := strings.SplitN(item, "=", 2)
		if len(parts) != 2 {
			continue
		}
		key := strings.TrimSpace(parts[0])
		value := strings.TrimSpace(parts[1])

		value = strings.TrimRight(value, "}")

		convertedRecords[key] = value
	}

	return convertedRecords, nil
}

func (e *EnSyncEngine) createRequest(ctx context.Context, command string, callback func(string) error) (string, error) {
	req, err := http.NewRequestWithContext(ctx, "POST", e.config.EnsyncURL+e.config.Client, strings.NewReader(command))
	if err != nil {
		return "", NewEnSyncError(err, "")
	}

	resp, err := e.client.Do(req)
	if err != nil {
		return "", NewEnSyncError(err, "")
	}
	defer resp.Body.Close()

	return e.processResponse(resp.Body, callback)
}

func (e *EnSyncEngine) processResponse(reader io.Reader, callback func(string) error) (string, error) {
	var data strings.Builder
	buf := make([]byte, 1024)

	for {
		n, err := reader.Read(buf)
		if n > 0 {
			chunk := string(buf[:n])
			data.WriteString(chunk)
			if callback != nil {
				if err := callback(chunk); err != nil {
					return "", err
				}
			}
		}
		if err != nil {
			if err != io.EOF {
				return "", NewEnSyncError(err, "")
			}
			break
		}
	}

	result := data.String()
	if strings.HasPrefix(result, "-FAIL:") {
		return "", NewEnSyncError(result, "EnSyncGenericError")
	}

	return result, nil
}

// Subscribe creates a subscription for events
func (c *EnSyncClient) Subscribe(ctx context.Context, eventName string) (*EnSyncSubscription, error) {
	command := fmt.Sprintf("SUB;CLIENT_ID=%s;EVENT_NAME=%s", c.clientId, eventName)
	_, err := c.engine.createRequest(ctx, command, nil)
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithCancel(ctx)
	return &EnSyncSubscription{
		client:  c,
		event:   eventName,
		context: ctx,
		cancel:  cancel,
	}, nil
}

// Pull starts pulling records from the subscription
func (s *EnSyncSubscription) Pull(ctx context.Context, opts EnSyncSubscribeOptions, handler func(EnSyncRecord) error) error {
	command := fmt.Sprintf("PULL;CLIENT_ID=%s;EVENT_NAME=%s", s.client.clientId, s.event)
	errCh := make(chan error, 1) // Buffered channel for error reporting

	go func() {
		defer close(errCh)

		for {
			select {
			case <-ctx.Done():
				errCh <- ctx.Err()
				return
			default:
				callback := func(chunk string) error {
					if ctx.Err() != nil {
						return ctx.Err()
					}

					if strings.HasPrefix(chunk, "-FAIL:") {
						return NewEnSyncError(chunk, "EnSyncGenericError")
					}

					if strings.HasPrefix(chunk, "+RECORD:") {
						chunk = strings.TrimPrefix(chunk, "+RECORD:")
						chunk = strings.TrimSuffix(chunk, "\n")

						content := convertKeyValueToJSON(chunk)

						var record EnSyncRecord
						if err := json.Unmarshal([]byte(content), &record); err != nil {
							return err
						}

						if err := handler(record); err != nil {
							return err
						}

						if opts.AutoAck {
							_, err := s.client.Ack(ctx, record.ID, record.Block)
							if err != nil {
								return err
							}
						}
					}
					return nil
				}

				retryDelay := time.Second
				for {
					_, err := s.client.engine.createRequest(ctx, command, callback)
					if err == nil {
						break
					}

					if ctx.Err() != nil {
						errCh <- ctx.Err()
						return
					}

					log.Printf("Retrying request after %v due to error: %v", retryDelay, err)
					time.Sleep(retryDelay)

					if retryDelay < 10*time.Second {
						retryDelay *= 2
					}
				}
			}
		}
	}()

	// Wait for error or cancellation
	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (c *EnSyncClient) Publish(ctx context.Context, eventName string, payload EnSyncEventPayload, opts EnSyncPublishOptions) error {
	data, err := json.Marshal(payload.Data)
	if err != nil {
		return NewEnSyncError(err, "")
	}

	fmt.Println(string(data))

	command := fmt.Sprintf("PUB;CLIENT_ID=%s;EVENT_NAME=%s;PAYLOAD=%s", c.clientId, eventName, string(data))
	_, err = c.engine.createRequest(ctx, command, nil)
	return err
}

func (c *EnSyncClient) Ack(ctx context.Context, id, block string) (string, error) {
	command := fmt.Sprintf("ACK;CLIENT_ID=%s;EVENT_IDEM=%s;BLOCK=%s", c.clientId, id, block)
	response, err := c.engine.createRequest(ctx, command, nil)
	return response, err
}

func (c *EnSyncClient) Rollback(ctx context.Context, id, block string) (string, error) {
	command := fmt.Sprintf("ROLLBACK;CLIENT_ID=%s;EVENT_IDEM=%s;BLOCK=%s", c.clientId, id, block)
	response, err := c.engine.createRequest(ctx, command, nil)
	return response, err
}

func (s *EnSyncSubscription) Unsubscribe() error {
	defer s.cancel()
	command := fmt.Sprintf("UNSUB;CLIENT_ID=%s;EVENT_NAME=%s", s.client.clientId, s.event)
	_, err := s.client.engine.createRequest(s.context, command, nil)
	return err
}

func (c *EnSyncClient) Close(ctx context.Context) error {
	command := fmt.Sprintf("CLOSE;CLIENT_ID=%s", c.clientId)
	_, err := c.engine.createRequest(ctx, command, nil)
	return err
}

func (e *EnSyncEngine) Close() error {
	e.internalAction.EndSession = true
	return nil
}

func convertKeyValueToJSON(data string) string {
	re := regexp.MustCompile(`(\w+)=`)
	data = re.ReplaceAllString(data, `"$1": `)
	re = regexp.MustCompile(`: (\w+)`)
	data = re.ReplaceAllString(data, `: "$1"`)
	return data
}

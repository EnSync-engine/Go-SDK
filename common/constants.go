package common

const (
	initialReconnectDelay      = 1  // seconds
	maxReconnectDelay          = 32 // seconds
	reconnectBackoffMultiplier = 2
)

const (
	workerTaskBufferMultiplier = 50
)

const (
	circuitBreakerThreshold        = 5
	circuitBreakerTimeout          = 60 // seconds
	circuitBreakerHalfOpenRequests = 3
)

const (
	defaultMaxRetries = 3
)

const (
	uuidByteLength = 16 // UUID requires 16 random bytes
)

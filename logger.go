package ensync

type Logger interface {
	Debug(msg string, keysAndValues ...interface{})
	Info(msg string, keysAndValues ...interface{})
	Warn(msg string, keysAndValues ...interface{})
	Error(msg string, keysAndValues ...interface{})
}

type noopLogger struct{}

func (n *noopLogger) Debug(msg string, kv ...interface{}) {}
func (n *noopLogger) Info(msg string, kv ...interface{})  {}
func (n *noopLogger) Warn(msg string, kv ...interface{})  {}
func (n *noopLogger) Error(msg string, kv ...interface{}) {}

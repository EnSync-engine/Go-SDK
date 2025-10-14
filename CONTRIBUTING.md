# Contributing to EnSync Go SDK

Thank you for your interest in contributing to the EnSync Go SDK! This document provides guidelines and instructions for contributing.

## Getting Started

### Prerequisites

- Go 1.21 or later
- Protocol Buffers compiler (`protoc`)
- Git

### Setup Development Environment

1. **Fork and clone the repository**

```bash
git clone https://github.com/your-username/Go-SDK.git
cd Go-SDK
```

2. **Install development tools**

```bash
make install-tools
```

3. **Download dependencies**

```bash
make deps
```

4. **Generate gRPC code**

```bash
make proto
```

## Development Workflow

### Making Changes

1. **Create a new branch**

```bash
git checkout -b feature/your-feature-name
```

2. **Make your changes**

Follow the coding standards outlined below.

3. **Format your code**

```bash
make fmt
```

4. **Run linters**

```bash
make lint
```

5. **Run tests**

```bash
make test
```

6. **Commit your changes**

```bash
git add .
git commit -m "feat: add your feature description"
```

### Commit Message Convention

We follow the [Conventional Commits](https://www.conventionalcommits.org/) specification:

- `feat:` - A new feature
- `fix:` - A bug fix
- `docs:` - Documentation changes
- `style:` - Code style changes (formatting, etc.)
- `refactor:` - Code refactoring
- `test:` - Adding or updating tests
- `chore:` - Maintenance tasks

Examples:
```
feat: add support for batch publishing
fix: resolve race condition in subscription handler
docs: update README with new examples
test: add unit tests for crypto module
```

## Coding Standards

### Go Style Guide

Follow the official [Go Code Review Comments](https://github.com/golang/go/wiki/CodeReviewComments) and [Effective Go](https://golang.org/doc/effective_go).

### Key Principles

1. **Idiomatic Go**: Write code that follows Go conventions
2. **Simplicity**: Prefer simple, readable code over clever solutions
3. **Error Handling**: Always handle errors explicitly
4. **Documentation**: Document all exported functions, types, and packages
5. **Testing**: Write tests for new functionality

### Code Examples

#### Good: Idiomatic Go

```go
// GetClientPublicKey returns the client's public key in base64 format.
func (e *GRPCEngine) GetClientPublicKey() string {
    return e.config.clientHash
}

// Publish publishes an event to the EnSync system.
func (e *GRPCEngine) Publish(eventName string, recipients []string, payload map[string]interface{}, metadata *EventMetadata, options *PublishOptions) (string, error) {
    if len(recipients) == 0 {
        return "", NewEnSyncError(RecipientsRequired, ErrTypeValidation, nil)
    }
    
    // Implementation...
    return eventID, nil
}
```

#### Bad: Non-idiomatic

```go
// Don't use getters with "Get" prefix for simple field access
func (e *GRPCEngine) GetConfig() Config {
    return e.config
}

// Don't panic in library code
func (e *GRPCEngine) Publish(...) string {
    if len(recipients) == 0 {
        panic("recipients required")
    }
    return eventID
}
```

### Documentation

All exported functions, types, and constants must have documentation comments:

```go
// Engine is the main interface for EnSync clients.
// It provides methods for publishing and subscribing to events.
type Engine interface {
    // CreateClient creates and authenticates a new client.
    CreateClient(accessKey string, options ...ClientOption) error
    
    // Publish publishes an event to the EnSync system.
    Publish(eventName string, recipients []string, payload map[string]interface{}, metadata *EventMetadata, options *PublishOptions) (string, error)
}
```

### Error Handling

Always return errors, never panic in library code:

```go
// Good
func (e *GRPCEngine) Connect() error {
    conn, err := grpc.Dial(e.config.url, opts...)
    if err != nil {
        return NewEnSyncError("connection failed", ErrTypeConnection, err)
    }
    return nil
}

// Bad
func (e *GRPCEngine) Connect() {
    conn, err := grpc.Dial(e.config.url, opts...)
    if err != nil {
        panic(err)
    }
}
```

### Concurrency

Use mutexes to protect shared state:

```go
type GRPCEngine struct {
    state struct {
        mu              sync.RWMutex
        isAuthenticated bool
    }
}

func (e *GRPCEngine) IsAuthenticated() bool {
    e.state.mu.RLock()
    defer e.state.mu.RUnlock()
    return e.state.isAuthenticated
}

func (e *GRPCEngine) setAuthenticated(value bool) {
    e.state.mu.Lock()
    defer e.state.mu.Unlock()
    e.state.isAuthenticated = value
}
```

## Testing

### Writing Tests

1. **Unit Tests**: Test individual functions and methods
2. **Integration Tests**: Test interactions between components
3. **Example Tests**: Provide runnable examples

#### Unit Test Example

```go
func TestAnalyzePayload(t *testing.T) {
    engine := &GRPCEngine{}
    
    payload := map[string]interface{}{
        "name":   "test",
        "count":  42,
        "active": true,
    }
    
    meta := engine.AnalyzePayload(payload)
    
    if meta.ByteSize == 0 {
        t.Error("expected non-zero byte size")
    }
    
    if meta.Skeleton["name"] != "string" {
        t.Errorf("expected string type, got %s", meta.Skeleton["name"])
    }
}
```

#### Example Test

```go
func Example_publish() {
    engine, _ := ensync.NewGRPCEngine("grpc://localhost:50051")
    defer engine.Close()
    
    engine.CreateClient("access-key")
    
    eventID, _ := engine.Publish(
        "test/event",
        []string{"recipient-key"},
        map[string]interface{}{"data": "value"},
        nil,
        nil,
    )
    
    fmt.Println("Event published:", eventID)
}
```

### Running Tests

```bash
# Run all tests
make test

# Run tests with coverage
make test-coverage

# Run specific test
go test -v -run TestAnalyzePayload
```

## Pull Request Process

1. **Update documentation** if you're changing functionality
2. **Add tests** for new features
3. **Ensure all tests pass**: `make check`
4. **Update CHANGELOG.md** with your changes
5. **Submit pull request** with a clear description

### Pull Request Template

```markdown
## Description
Brief description of the changes

## Type of Change
- [ ] Bug fix
- [ ] New feature
- [ ] Breaking change
- [ ] Documentation update

## Testing
Describe the tests you ran

## Checklist
- [ ] Code follows style guidelines
- [ ] Self-review completed
- [ ] Comments added for complex code
- [ ] Documentation updated
- [ ] Tests added/updated
- [ ] All tests pass
```

## Code Review

All submissions require review. We use GitHub pull requests for this purpose.

### Review Criteria

- Code quality and style
- Test coverage
- Documentation completeness
- Performance considerations
- Security implications

## Release Process

Maintainers will handle releases following semantic versioning:

- **MAJOR**: Breaking changes
- **MINOR**: New features (backward compatible)
- **PATCH**: Bug fixes (backward compatible)

## Questions?

- Open an issue for bugs or feature requests
- Join our community discussions
- Check existing issues and PRs first

## License

By contributing, you agree that your contributions will be licensed under the ISC License.

## Thank You!

Your contributions make this project better for everyone. Thank you for taking the time to contribute!

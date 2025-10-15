# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added
- Initial Go SDK implementation with unified API
- Two-phase connection model (transport connection + EnSync authentication)
- gRPC client support with streaming subscriptions
- WebSocket client support with auto-reconnection
- Protocol auto-detection from URL schemes
- End-to-end encryption with Ed25519/Curve25519
- Hybrid encryption for multiple recipients
- Event publishing and subscribing with full event lifecycle
- Event management (acknowledge, defer, discard, replay, rollback)
- Flow control (pause, resume subscription)
- Automatic heartbeat for gRPC connections
- Automatic ping/pong keepalive for WebSocket connections
- Connection resilience with retry logic and reconnection
- Comprehensive error handling with typed EnSyncError
- Interface-based design for testability and mocking
- Functional options pattern for flexible configuration
- Thread-safe operations with proper mutex protection
- Context support for cancellation and timeouts
- Complete example applications for both protocols
- Comprehensive documentation with multiple learning paths
- Unit test examples and integration patterns
- Makefile for build automation and common tasks
- Contributing guidelines with coding standards
- Mock server testing infrastructure for gRPC client/server interactions

### Improved
- **WebSocket Engine Refactoring**: Major improvements to connection and authentication logic
  - Enhanced message handling with better error recovery
  - Improved publish/subscribe method implementations
  - Better subscription management and lifecycle handling
  - Streamlined connection state management
- **WebSocket URL Parsing**: Added standardized `parseWebSocketURL` function for consistent endpoint handling
- **Testing Infrastructure**: New `SimpleMockGRPCServer` for comprehensive gRPC testing scenarios

### Features
- **Unified API**: Single import, protocol-agnostic interface
- **Dual Transport**: gRPC and WebSocket with same API
- **Two-Phase Connection**: Clear separation of transport and authentication
- **Security**: Ed25519 encryption with hybrid multi-recipient mode
- **Reliability**: Auto-reconnection, heartbeat, and comprehensive error handling
- **Flexibility**: Interface-based design with functional options
- **Type Safety**: Strong typing with Go structs and interfaces
- **Concurrency**: Goroutine-based event handling with proper synchronization
- **Idiomatic Go**: Follows Go best practices and conventions
- **Production Ready**: Context support, graceful shutdown, resource cleanup

### Architecture
- **Clean Separation**: Transport layer (WebSocket/gRPC) vs EnSync protocol layer
- **Interface Design**: Engine and Subscription interfaces for flexibility
- **Package Structure**: Unified ensync package with internal implementations
- **Error Handling**: Typed errors with proper error chaining
- **Resource Management**: Explicit cleanup with defer and context cancellation
- **Connection Flow**: Connection established in constructor, authentication in CreateClient

### Examples
- Basic publisher and subscriber examples
- gRPC-specific examples with streaming
- WebSocket examples with reconnection
- Advanced subscription patterns (manual ack, defer, pause/resume)
- Protocol auto-detection examples
- Error handling and retry patterns

### Documentation
- Complete README with API reference
- Quick start guide for immediate usage
- Comprehensive getting started tutorial
- Design document explaining Go patterns
- File structure guide for contributors
- Contributing guidelines with coding standards
- Example tests serving as documentation

### Fixed
- ❌ Slice pointer bug in gRPC publisher (responses were lost)
- ❌ Uninitialized subscriptionMgr causing nil pointer panics  
- ❌ Encryption function inconsistency between WebSocket and gRPC
- ❌ Incorrect recipient key handling in encryption
- ❌ Message handler goroutine management and cleanup
- ❌ Connection state management and thread safety
- ❌ WebSocket reconnection logic and error handling

## [0.1.0] - 2025-10-15

### Added
- Initial release of EnSync Go SDK
- gRPC and WebSocket transport protocols  
- Basic publishing and subscribing functionality
- Ed25519 encryption support
- Example applications
- Core documentation

### Architecture Decisions
- Chose unified API over separate packages
- Implemented two-phase connection model
- Used interface-based design for testability
- Applied functional options pattern for configuration
- Followed idiomatic Go patterns throughout

[Unreleased]: https://github.com/EnSync-engine/Go-SDK/compare/v0.1.0...HEAD
[0.1.0]: https://github.com/EnSync-engine/Go-SDK/releases/tag/v0.1.0
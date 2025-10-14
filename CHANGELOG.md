# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added
- Initial Go SDK implementation
- gRPC client support with streaming
- WebSocket client support
- End-to-end encryption with Ed25519/Curve25519
- Hybrid encryption for multiple recipients
- Event publishing and subscribing
- Event management (acknowledge, defer, discard, replay)
- Flow control (pause, resume)
- Automatic heartbeat for gRPC
- Automatic ping/pong for WebSocket
- Comprehensive error handling with typed errors
- Interface-based design for testability
- Functional options pattern for configuration
- Thread-safe operations with mutex protection
- Context support for cancellation
- Example applications for gRPC and WebSocket
- Comprehensive documentation
- Unit test examples
- Makefile for common tasks
- Contributing guidelines

### Features
- **Dual Transport**: gRPC and WebSocket support
- **Security**: Ed25519 encryption with hybrid mode
- **Reliability**: Auto-reconnection and heartbeat
- **Flexibility**: Interface-based design
- **Type Safety**: Strong typing with Go structs
- **Concurrency**: Goroutine-based event handling
- **Idiomatic**: Follows Go best practices

## [0.1.0] - 2025-10-14

### Added
- Initial release of EnSync Go SDK
- gRPC and WebSocket transport protocols
- Comprehensive examples and documentation

[Unreleased]: https://github.com/EnSync-engine/Go-SDK/compare/v0.1.0...HEAD
[0.1.0]: https://github.com/EnSync-engine/Go-SDK/releases/tag/v0.1.0

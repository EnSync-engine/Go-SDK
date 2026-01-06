# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

## [0.1.6] - 2026-01-06

### Added
- Version 0.1.6 release


## [0.1.5] - 2026-01-06

### Added
- Version 0.1.5 release


## [0.1.4] - 2026-01-06

### Added
- Version 0.1.4 release


## [0.1.3] - 2025-10-16

### Added
- Version 0.1.3 release


## [v1.2.1] - 2025-10-16

### 🔒 Security & Cryptography
- **MAJOR**: Refactored cryptographic implementation to use proven libraries
- Replaced custom Ed25519 ↔ Curve25519 conversion with `filippo.io/edwards25519` library
- Implemented RFC 7748 compliant hash-based private key conversion
- Fixed authentication failures in event subscription and publishing
- Removed dangerous custom cryptographic code in favor of standard implementations

### 🧹 Code Quality
- Fixed magic number violations by adding proper cryptographic constants
- Removed unused `nonceSize` constant (re-added with proper usage)
- Cleaned up legacy `ed25519PrivateKeyToCurve25519` wrapper function
- Improved error handling for gRPC stream cancellations
- Enhanced logging to distinguish expected context cancellations from actual errors

### 🐛 Bug Fixes
- Fixed decryption failures caused by improper key conversion
- Resolved gRPC stream error logging format issues
- Fixed "Stream receive error: context canceled" being logged as errors instead of info

### 🧪 Testing
- Updated all crypto tests to use proper Ed25519 key generation
- Removed unused imports and cleaned up test dependencies
- All cryptographic unit tests now pass with library-based implementation

### 📚 Documentation
- Updated code comments to reflect new cryptographic approach
- Added proper error handling documentation for gRPC streams
- Fixed test compatibility with library-based cryptographic functions
- All unit tests now pass with the new implementation
- Removed dependency on `golang.org/x/crypto/nacl/box` from tests

### 📚 Documentation
- Updated code comments to reflect library-based approach
- Added proper error messages with context for debugging

## [0.1.2] - 2025-10-16

### Added
- Version 0.1.2 release


## [0.1.1] - 2025-10-16

### Changed
- **BREAKING**: Moved protobuf definitions to internal package
- Relocated proto files from `proto/` to `internal/proto/` directory
- Updated all import paths from `github.com/EnSync-engine/Go-SDK/proto` to `github.com/EnSync-engine/Go-SDK/internal/proto`
- External users can no longer directly access protobuf types (by design - following Go best practices)

### Improved
- **Developer Experience**: External SDK users no longer need protobuf knowledge, toolchain, or dependencies
- **API Cleanliness**: Public API exclusively exposes clean Go interfaces and structs
- **Encapsulation**: Internal package pattern prevents accidental access to implementation details
- **Build Simplicity**: External projects require no `protoc`, protobuf Go packages, or `.proto` file handling

### Technical Details
- Updated internal imports from `proto.` to `pb.` namespace throughout codebase
- Modified Makefile `proto` target to generate from `internal/proto/ensync.proto`
- Added generated `*.pb.go` files to `.gitignore` while preserving source `.proto` files in version control
- Regenerated protobuf code using `protoc` in new internal location
- Updated affected files: `grpc/engine.go`, `grpc/client.go`, `mock_servers_test.go`
- Verified external usage example requires zero protobuf dependencies

### Impact
- **Breaking Change**: Internal protobuf types no longer accessible to external users
- **Cleaner Integration**: External users only import main SDK package
- **Future-Proof**: Internal implementation can evolve without affecting public API contract

## [0.1.0] - 2025-10-15

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

### Fixed
- Slice pointer bug in gRPC publisher (responses were lost)
- Uninitialized subscriptionMgr causing nil pointer panics  
- Encryption function inconsistency between WebSocket and gRPC
- Incorrect recipient key handling in encryption
- Message handler goroutine management and cleanup
- Connection state management and thread safety
- WebSocket reconnection logic and error handling

[Unreleased]: https://github.com/EnSync-engine/Go-SDK/compare/v0.1.1...HEAD
[0.1.1]: https://github.com/EnSync-engine/Go-SDK/compare/v0.1.0...v0.1.1
[0.1.0]: https://github.com/EnSync-engine/Go-SDK/releases/tag/v0.1.0
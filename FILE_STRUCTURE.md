# Go SDK File Structure

```
Go-SDK/
│
├── � common/                  # Shared utilities and types
│   ├── base_engine.go          # Base engine functionality
│   ├── circuit_breaker.go      # Circuit breaker implementation
│   ├── crypto.go              # Encryption/decryption utilities
│   ├── errors.go              # Error types and handling
│   ├── logger.go              # Logging utilities
│   ├── options.go             # Configuration options
│   ├── retry.go               # Retry logic
│   ├── subscription.go        # Subscription management
│   └── types.go               # Core types and interfaces
│
├── 📁 grpc/                   # gRPC transport implementation
│   ├── engine.go              # gRPC engine
│   └── options.go             # gRPC-specific options
│
├── 📁 websocket/              # WebSocket transport implementation
│   ├── engine.go              # WebSocket engine
│   └── options.go             # WebSocket-specific options
│
├── 📁 proto/                  # Protocol buffer definitions
│   ├── ensync.proto           # Protocol definitions
│   ├── ensync.pb.go           # Generated Go code
│   └── ensync_grpc.pb.go      # Generated gRPC code
│
├── 📄 example_test.go          # Example usage tests
│
├── 📚 Documentation
│   ├── README.md              # User documentation
│   ├── QUICKSTART.md          # Quick start guide
│   ├── GETTING_STARTED.md     # Comprehensive beginner's guide 
│   ├── INDEX.md               # Documentation navigation
│   ├── DESIGN.md              # Design decisions and patterns
│   ├── CONTRIBUTING.md        # Contribution guidelines
│   ├── CHANGELOG.md           # Version history
│   └── FILE_STRUCTURE.md      # This file
│
├── 🔧 Configuration Files
│   ├── go.mod                 # Go module definition
│   ├── go.sum                 # Go checksum file
│   ├── Makefile               # Build automation
│   ├── generate.sh            # gRPC code generation script
│   └── LICENSE                # ISC License
│
└── 💡 Examples
    ├── grpc_publisher/
    │   └── main.go            # gRPC publisher example
    ├── grpc_subscriber/
    │   └── main.go            # gRPC subscriber example
    └── websocket_example/
        └── main.go            # WebSocket client example
```

## File Categories

### 🎯 Core Implementation

#### Common Package
| File | Purpose | Dependencies |
|------|---------|--------------|
| `common/errors.go` | Error handling | Standard library |
| `common/crypto.go` | Encryption | `golang.org/x/crypto` |
| `common/types.go` | Interfaces & types | Standard library |
| `common/base_engine.go` | Base engine functionality | Standard library |
| `common/circuit_breaker.go` | Circuit breaker | Standard library |
| `common/logger.go` | Logging utilities | `go.uber.org/zap` |
| `common/options.go` | Configuration options | Standard library |
| `common/retry.go` | Retry logic | Standard library |
| `common/subscription.go` | Subscription management | Standard library |

#### Transport Implementations
| File | Purpose | Dependencies |
|------|---------|--------------|
| `grpc/engine.go` | gRPC engine | `google.golang.org/grpc` |
| `grpc/options.go` | gRPC options | Standard library |
| `websocket/engine.go` | WebSocket engine | `github.com/gorilla/websocket` |
| `websocket/options.go` | WebSocket options | Standard library |

#### Protocol Buffers
| File | Purpose | Dependencies |
|------|---------|--------------|
| `proto/ensync.proto` | Protocol definitions | - |

#### Tests & Examples
| File | Purpose | Dependencies |
|------|---------|--------------|
| `example_test.go` | Examples | All packages |

### 📖 Documentation


| File | Purpose | Audience |
|------|---------|----------|
| `README.md` | Main documentation | End users |
| `QUICKSTART.md` | Quick start guide | New users |
| `GETTING_STARTED.md` | Comprehensive guide | Beginners |
| `INDEX.md` | Documentation navigation | All users |
| `DESIGN.md` | Design decisions | Developers |
| `CONTRIBUTING.md` | Contribution guide | Contributors |
| `CHANGELOG.md` | Version history | All |
| `FILE_STRUCTURE.md` | This file | All |

### ⚙️ Configuration

| File | Purpose |
|------|---------|
| `go.mod` | Go module definition |
| `go.sum` | Go checksum file |
| `Makefile` | Build automation |
| `generate.sh` | gRPC code generation |
| `LICENSE` | ISC License |

### 🎨 Examples

| Example | Type | Purpose |
|---------|------|---------|
| `grpc_publisher` | Publisher | Publish events via gRPC |
| `grpc_subscriber` | Subscriber | Subscribe to events via gRPC |
| `websocket_example` | Full client | WebSocket client example |

## Code Organization

### Package Structure

```
Go-SDK/
├── common/ (shared utilities)
│   ├── Engine interface
│   ├── Subscription interface  
│   ├── Error types
│   ├── Crypto functions
│   ├── Retry logic
│   ├── Circuit breaker
│   ├── Logger utilities
│   └── Configuration options
│
├── grpc/ (gRPC transport)
│   ├── GRPCEngine implementation
│   ├── gRPC-specific options
│   └── Protocol buffer integration
│
├── websocket/ (WebSocket transport)
│   ├── WebSocketEngine implementation
│   ├── WebSocket-specific options
│   └── Message handling
│
└── proto/ (generated code)
    ├── Protocol definitions
    └── Generated gRPC code
```

### Dependency Graph

```
examples/
    ├─> github.com/EnSync-engine/Go-SDK/grpc
    ├─> github.com/EnSync-engine/Go-SDK/websocket
    └─> github.com/EnSync-engine/Go-SDK/common

grpc/
    ├─> github.com/EnSync-engine/Go-SDK/common
    ├─> github.com/EnSync-engine/Go-SDK/proto
    ├─> google.golang.org/grpc
    └─> google.golang.org/protobuf

websocket/
    ├─> github.com/EnSync-engine/Go-SDK/common  
    └─> github.com/gorilla/websocket

common/
    ├─> golang.org/x/crypto
    └─> go.uber.org/zap
```

## Key Files by Use Case

### Getting Started
1. `QUICKSTART.md` - Start here
2. `GETTING_STARTED.md` - Comprehensive guide
3. `README.md` - Full documentation
4. `INDEX.md` - Documentation navigation
5. `examples/grpc_subscriber/main.go` - Working example

### Understanding Design
1. `DESIGN.md` - Design patterns
2. `common/types.go` - Core interfaces
3. `common/base_engine.go` - Base functionality

### Contributing
1. `CONTRIBUTING.md` - Guidelines
2. `Makefile` - Build commands
3. `CHANGELOG.md` - Version history

### Implementation
1. `grpc/engine.go` - gRPC implementation
2. `websocket/engine.go` - WebSocket implementation
3. `common/crypto.go` - Encryption utilities
4. `common/subscription.go` - Subscription management

## Build Artifacts (Generated)

These files are generated and not in version control:

```
proto/
├── ensync.pb.go         
└── ensync_grpc.pb.go     

bin/                      
├── grpc_publisher
├── grpc_subscriber
└── websocket_example

coverage.out              
coverage.html             
```

## Import Paths

```go
// Common utilities and types
import "github.com/EnSync-engine/Go-SDK/common"

// gRPC engine
import ensync "github.com/EnSync-engine/Go-SDK/grpc"

// WebSocket engine  
import ensync "github.com/EnSync-engine/Go-SDK/websocket"

// Protocol buffers (generated code)
import pb "github.com/EnSync-engine/Go-SDK/proto"
```

## File Relationships

```
go.mod
  └─> Defines module and dependencies

common/
  ├─> errors.go (used by all implementation files)
  ├─> crypto.go (used by engine implementations)
  ├─> types.go (defines interfaces)
  ├─> base_engine.go (shared engine functionality)
  ├─> subscription.go (subscription management)
  ├─> circuit_breaker.go (reliability)
  ├─> retry.go (retry logic)
  ├─> logger.go (logging utilities)
  └─> options.go (configuration)

grpc/
  ├─> engine.go (imports common/*)
  ├─> options.go (gRPC-specific config)
  └─> Uses: proto/ensync.proto (generated code)

websocket/
  ├─> engine.go (imports common/*)
  ├─> options.go (WebSocket-specific config)
  └─> Uses: github.com/gorilla/websocket

proto/
  ├─> ensync.proto (protocol definitions)
  ├─> ensync.pb.go (generated)
  └─> ensync_grpc.pb.go (generated)

example_test.go
  └─> Imports: grpc/, websocket/, common/

examples/*/main.go
  └─> Imports: grpc/ or websocket/ + common/
```

## Maintenance Guide

### To Add a New Feature

1. Update interfaces in `common/types.go`
2. Update base functionality in `common/base_engine.go` if needed
3. Implement in `grpc/engine.go` and/or `websocket/engine.go`
4. Add tests in `example_test.go`
5. Update `README.md` and `QUICKSTART.md`
6. Add example in `examples/`
7. Update `CHANGELOG.md`

### To Fix a Bug

1. Identify affected file(s)
2. Write test to reproduce
3. Fix implementation
4. Verify test passes
5. Update `CHANGELOG.md`

### To Update Documentation

1. Edit relevant `.md` file
2. Ensure consistency across all docs
3. Update examples if needed
4. Run `make fmt` for code examples

## Quick Reference

### Most Important Files

1. **`README.md`** - Start here for usage
2. **`QUICKSTART.md`** - Quick start guide  
3. **`INDEX.md`** - Documentation navigation
4. **`common/types.go`** - Core interfaces
5. **`grpc/engine.go`** - Main gRPC implementation
6. **`websocket/engine.go`** - Main WebSocket implementation

### Build Commands

```bash
make help              # Show all commands
make proto             # Generate gRPC code
make test              # Run tests
make build-examples    # Build examples
make check             # Run all checks
```

### File Naming Conventions

- `*.go` - Go source files
- `*_test.go` - Test files
- `*.md` - Markdown documentation
- `*.proto` - Protocol buffer definitions
- `Makefile` - Build automation
- `*.sh` - Shell scripts
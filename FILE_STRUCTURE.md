# Go SDK File Structure

```
Go-SDK/
│
├── 📄 Core Go Files (2,400+ lines)
│   ├── errors.go              # Error types and handling (60 lines)
│   ├── crypto.go              # Encryption/decryption utilities (380 lines)
│   ├── types.go               # Core types and interfaces (150 lines)
│   ├── grpc_client.go         # gRPC client implementation (850 lines)
│   ├── websocket_client.go    # WebSocket client implementation (750 lines)
│   └── example_test.go        # Example usage tests (200 lines)
│
├── 📚 Documentation
│   ├── README.md              # User documentation
│   ├── QUICKSTART.md          # Quick start guide
│   ├── DESIGN.md              # Design decisions and patterns
│   ├── CONTRIBUTING.md        # Contribution guidelines
│   ├── PORTING_SUMMARY.md     # Detailed porting notes
│   ├── PROJECT_SUMMARY.md     # Project completion summary
│   ├── CHANGELOG.md           # Version history
│
├── 🔧 Configuration Files
│   ├── go.mod                 # Go module definition
│   ├── Makefile               # Build automation
│   ├── generate.sh            # gRPC code generation script
│   ├── .gitignore             # Git ignore rules
│   └── LICENSE                # ISC License
│
├── 📦 Protocol Buffers
│   └── proto/
│       └── ensync.proto       # gRPC service definitions
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

| File | Purpose | Dependencies |
|------|---------|--------------|
| `errors.go` | Error handling | Standard library |
| `crypto.go` | Encryption | `golang.org/x/crypto` |
| `types.go` | Interfaces & types | Standard library |
| `grpc_client.go` | gRPC client | `google.golang.org/grpc` |
| `websocket_client.go` | WebSocket client | `github.com/gorilla/websocket` |
| `example_test.go` | Examples | Core files |

### 📖 Documentation


| File | Purpose | Audience |
|------|---------|----------|
| `README.md` | Main documentation | End users |
| `QUICKSTART.md` | Quick start guide | New users |
| `DESIGN.md` | Design decisions | Developers |
| `CONTRIBUTING.md` | Contribution guide | Contributors |
| `PORTING_SUMMARY.md` | Porting details | Developers |
| `PROJECT_SUMMARY.md` | Project overview | All |
| `CHANGELOG.md` | Version history | All |
| `FILE_STRUCTURE.md` | This file | All |

### ⚙️ Configuration

| File | Purpose |
|------|---------|
| `go.mod` | Go module definition |
| `Makefile` | Build automation |
| `generate.sh` | gRPC code generation |
| `.gitignore` | Git ignore rules |
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
ensync (main package)
├── Public API
│   ├── Engine interface
│   ├── Subscription interface
│   ├── NewGRPCEngine()
│   ├── NewWebSocketEngine()
│   └── Error types
│
├── gRPC Implementation
│   ├── GRPCEngine struct
│   ├── grpcSubscription struct
│   └── Helper functions
│
├── WebSocket Implementation
│   ├── WebSocketEngine struct
│   ├── wsSubscription struct
│   └── Helper functions
│
└── Shared Utilities
    ├── Crypto functions
    ├── Error handling
    └── Type definitions
```

### Dependency Graph

```
examples/
    └─> ensync (core package)
            ├─> google.golang.org/grpc
            ├─> github.com/gorilla/websocket
            ├─> golang.org/x/crypto
            └─> google.golang.org/protobuf
```

## Key Files by Use Case

### Getting Started
1. `QUICKSTART.md` - Start here
2. `README.md` - Full documentation
3. `examples/grpc_subscriber/main.go` - Working example

### Understanding Design
1. `DESIGN.md` - Design patterns
3. `types.go` - Core interfaces

### Contributing
1. `CONTRIBUTING.md` - Guidelines
2. `Makefile` - Build commands
3. `PROJECT_SUMMARY.md` - Project overview

### Implementation
1. `grpc_client.go` - gRPC implementation
2. `websocket_client.go` - WebSocket implementation
3. `crypto.go` - Encryption utilities

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
// Main package
import ensync "github.com/EnSync-engine/Go-SDK"

// Protocol buffers (after generation)
import pb "github.com/EnSync-engine/Go-SDK/proto"
```

## File Relationships

```
go.mod
  └─> Defines module and dependencies

errors.go
  └─> Used by all implementation files

crypto.go
  └─> Used by grpc_client.go and websocket_client.go

types.go
  ├─> Defines interfaces implemented by:
  │   ├─> grpc_client.go (Engine, Subscription)
  │   └─> websocket_client.go (Engine, Subscription)
  └─> Used by all files

grpc_client.go
  ├─> Imports: types.go, errors.go, crypto.go
  └─> Uses: proto/ensync.proto (generated code)

websocket_client.go
  ├─> Imports: types.go, errors.go, crypto.go
  └─> Uses: github.com/gorilla/websocket

example_test.go
  └─> Imports: all core files

examples/*/main.go
  └─> Imports: ensync package
```

## Maintenance Guide

### To Add a New Feature

1. Update interfaces in `types.go`
2. Implement in `grpc_client.go` and/or `websocket_client.go`
3. Add tests in `example_test.go`
4. Update `README.md`
5. Add example in `examples/`
6. Update `CHANGELOG.md`

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
2. **`types.go`** - Core interfaces
3. **`grpc_client.go`** - Main gRPC implementation
4. **`QUICKSTART.md`** - Quick start guide

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
# EnSync Go SDK - Documentation Index

Welcome to the EnSync Go SDK documentation! This index will help you find the right documentation for your needs.

## 🎯 Start Here

### New to EnSync?
1. **[GETTING_STARTED.md](GETTING_STARTED.md)** - Complete beginner's guide
2. **[QUICKSTART.md](QUICKSTART.md)** - 5-minute quick start
3. **[README.md](README.md)** - Full user documentation

### Experienced Developer?
1. **[README.md](README.md)** - API reference and examples
2. **[DESIGN.md](DESIGN.md)** - Architecture and patterns
3. **[examples/](examples/)** - Working code examples

## 📚 Documentation by Purpose

### Getting Started
| Document | Purpose |
|----------|---------|
| [GETTING_STARTED.md](GETTING_STARTED.md) | Complete beginner's guide with patterns |
| [QUICKSTART.md](QUICKSTART.md) | Minimal quick start guide |
| [README.md](README.md) | Comprehensive user documentation |

### Understanding the SDK
| Document | Purpose |
|----------|---------|
| [DESIGN.md](DESIGN.md) | Design decisions and Go patterns |
| [FILE_STRUCTURE.md](FILE_STRUCTURE.md) | Project structure and organization |

### Contributing
| Document | Purpose |
|----------|---------|
| [CONTRIBUTING.md](CONTRIBUTING.md) | Contribution guidelines |
| [PROJECT_SUMMARY.md](PROJECT_SUMMARY.md) | Project overview and status |
| [CHANGELOG.md](CHANGELOG.md) | Version history |

## 🗂️ Documentation by Topic

### Installation & Setup
- [GETTING_STARTED.md](GETTING_STARTED.md#-installation) - Installation steps
- [QUICKSTART.md](QUICKSTART.md#installation) - Quick installation
- [README.md](README.md#installation) - Detailed installation

### Basic Usage
- [GETTING_STARTED.md](GETTING_STARTED.md#-your-first-event) - First event tutorial
- [QUICKSTART.md](QUICKSTART.md#basic-usage) - Basic examples
- [README.md](README.md#quick-start) - Quick start examples

### API Reference
- [README.md](README.md#api-reference) - Complete API documentation
- [types.go](types.go) - Interface definitions
- [example_test.go](example_test.go) - Example tests

### Transport Protocols
- [README.md](README.md#transport-options) - gRPC vs WebSocket
- [GETTING_STARTED.md](GETTING_STARTED.md#-choosing-a-transport) - Transport guide
- [grpc_client.go](grpc_client.go) - gRPC implementation
- [websocket_client.go](websocket_client.go) - WebSocket implementation

### Event Management
- [README.md](README.md#event-management) - Event operations
- [GETTING_STARTED.md](GETTING_STARTED.md#-common-patterns) - Common patterns
- [examples/](examples/) - Working examples

### Error Handling
- [README.md](README.md#error-handling) - Error handling guide
- [GETTING_STARTED.md](GETTING_STARTED.md#-debugging) - Debugging tips
- [errors.go](errors.go) - Error types

### Security & Encryption
- [README.md](README.md#features) - Security features
- [GETTING_STARTED.md](GETTING_STARTED.md#-working-with-encryption) - Encryption guide
- [crypto.go](crypto.go) - Cryptography implementation

### Design & Architecture
- [DESIGN.md](DESIGN.md) - Complete design documentation
- [FILE_STRUCTURE.md](FILE_STRUCTURE.md) - File organization

### Building & Testing
- [Makefile](Makefile) - Build commands
- [CONTRIBUTING.md](CONTRIBUTING.md#development-workflow) - Development workflow
- [generate.sh](generate.sh) - gRPC code generation

## 🎓 Learning Paths

### Path 1: Quick Start 
1. [QUICKSTART.md](QUICKSTART.md)
2. [examples/grpc_subscriber/main.go](examples/grpc_subscriber/main.go)
3. [examples/grpc_publisher/main.go](examples/grpc_publisher/main.go)
4. Build your first app

### Path 2: Comprehensive Learning
1. [GETTING_STARTED.md](GETTING_STARTED.md)
2. [README.md](README.md)
3. [DESIGN.md](DESIGN.md)
4. [examples/](examples/)
5. Practice exercises

### Path 3: Deep Dive (4 hours)
1. [GETTING_STARTED.md](GETTING_STARTED.md)
2. [README.md](README.md)
3. [DESIGN.md](DESIGN.md)
4. [FILE_STRUCTURE.md](FILE_STRUCTURE.md)
5. Source code review
6. Build complex examples

### Path 4: Contributor 
1. [README.md](README.md)
2. [DESIGN.md](DESIGN.md)
3. [CONTRIBUTING.md](CONTRIBUTING.md)
4. [PROJECT_SUMMARY.md](PROJECT_SUMMARY.md)
5. [FILE_STRUCTURE.md](FILE_STRUCTURE.md)
6. Source code review
7. Setup dev environment

## 📖 Documentation Files

### User Documentation
- **[README.md](README.md)** - Main documentation
- **[QUICKSTART.md](QUICKSTART.md)** - Quick start guide
- **[GETTING_STARTED.md](GETTING_STARTED.md)** - Beginner's guide
- **[INDEX.md](INDEX.md)** - This file (documentation index)

### Developer Documentation
- **[DESIGN.md](DESIGN.md)** - Design decisions
- **[FILE_STRUCTURE.md](FILE_STRUCTURE.md)** - File organization
- **[PROJECT_SUMMARY.md](PROJECT_SUMMARY.md)** - Project overview
- **[CONTRIBUTING.md](CONTRIBUTING.md)** - Contribution guide

### Reference Documentation (2 files)
- **[CHANGELOG.md](CHANGELOG.md)** - Version history
- **[LICENSE](LICENSE)** - ISC License

## 🔍 Find Information By...

### By User Type

**Beginner Developer**
- Start: [GETTING_STARTED.md](GETTING_STARTED.md)
- Next: [QUICKSTART.md](QUICKSTART.md)
- Then: [examples/](examples/)

**Experienced Go Developer**
- Start: [README.md](README.md)
- Next: [DESIGN.md](DESIGN.md)
- Then: Source code

**Contributor**
- Start: [CONTRIBUTING.md](CONTRIBUTING.md)
- Next: [DESIGN.md](DESIGN.md)
- Then: [PROJECT_SUMMARY.md](PROJECT_SUMMARY.md)

### By Task

**Installing the SDK**
→ [GETTING_STARTED.md#installation](GETTING_STARTED.md#-installation)

**Publishing Events**
→ [README.md#publishing-events](README.md#publishing-events)

**Subscribing to Events**
→ [README.md#subscribing-to-events](README.md#subscribing-to-events)

**Handling Errors**
→ [README.md#error-handling](README.md#error-handling)

**Understanding Design**
→ [DESIGN.md](DESIGN.md)

**Contributing Code**
→ [CONTRIBUTING.md](CONTRIBUTING.md)

**Building Examples**
→ [Makefile](Makefile) + [examples/](examples/)

**Troubleshooting**
→ [GETTING_STARTED.md#-troubleshooting](GETTING_STARTED.md#-troubleshooting)

## 🛠️ Code Examples

### Example Files
- [examples/grpc_publisher/main.go](examples/grpc_publisher/main.go) - gRPC publisher
- [examples/grpc_subscriber/main.go](examples/grpc_subscriber/main.go) - gRPC subscriber
- [examples/websocket_example/main.go](examples/websocket_example/main.go) - WebSocket client
- [example_test.go](example_test.go) - Example tests

### Example Patterns
- [GETTING_STARTED.md#-common-patterns](GETTING_STARTED.md#-common-patterns) - Common patterns
- [README.md#complete-examples](README.md#complete-examples) - Complete examples
- [QUICKSTART.md#complete-example](QUICKSTART.md#complete-example) - Quick example

## 📦 Source Code

### Core Files
- [errors.go](errors.go) - Error handling
- [crypto.go](crypto.go) - Encryption/decryption
- [types.go](types.go) - Interfaces and types
- [grpc_client.go](grpc_client.go) - gRPC implementation
- [websocket_client.go](websocket_client.go) - WebSocket implementation

### Configuration Files
- [go.mod](go.mod) - Go module definition
- [Makefile](Makefile) - Build automation
- [generate.sh](generate.sh) - Code generation
- [.gitignore](.gitignore) - Git ignore rules

### Protocol Buffers
- [proto/ensync.proto](proto/ensync.proto) - gRPC definitions

## 🎯 Quick Links

### Most Important Documents
1. **[README.md](README.md)** - Start here for usage
2. **[QUICKSTART.md](QUICKSTART.md)** - 5-minute guide
3. **[DESIGN.md](DESIGN.md)** - Understand the design
4. **[examples/](examples/)** - See it in action

### Most Common Tasks
- **Install**: [GETTING_STARTED.md#installation](GETTING_STARTED.md#-installation)
- **Publish**: [README.md#publishing-events](README.md#publishing-events)
- **Subscribe**: [README.md#subscribing-to-events](README.md#subscribing-to-events)
- **Build**: [Makefile](Makefile)
- **Contribute**: [CONTRIBUTING.md](CONTRIBUTING.md)

### External Resources
- **EnSync Cloud**: https://ensync.cloud
- **Documentation**: https://docs.tryensync.com
- **GitHub**: https://github.com/EnSync-engine/Go-SDK

## 🔄 Documentation Updates

This documentation is maintained alongside the code. When contributing:

1. Update relevant documentation files
2. Keep examples in sync with code
3. Update CHANGELOG.md
4. Ensure consistency across all docs

See [CONTRIBUTING.md](CONTRIBUTING.md) for details.

## 💡 Tips for Using This Documentation

1. **Start with your goal** - Use the "By Task" section
2. **Follow a learning path** - Choose based on your experience
3. **Use examples** - Learn by doing
4. **Search within files** - Use Cmd+F / Ctrl+F
5. **Check the changelog** - Stay updated with changes

## 📞 Getting Help

Can't find what you're looking for?

1. **Search the docs** - Use this index
2. **Check examples** - See [examples/](examples/)
3. **Read the FAQ** - In [README.md](README.md)
4. **Ask questions** - GitHub Issues
5. **Read source** - Code is well-commented

## ✅ Documentation Checklist

Before you start coding, make sure you've read:

- [ ] [QUICKSTART.md](QUICKSTART.md) or [GETTING_STARTED.md](GETTING_STARTED.md)
- [ ] [README.md](README.md) - At least the API Reference section
- [ ] At least one example from [examples/](examples/)

Before contributing, make sure you've read:

- [ ] [CONTRIBUTING.md](CONTRIBUTING.md)
- [ ] [DESIGN.md](DESIGN.md)
- [ ] [PROJECT_SUMMARY.md](PROJECT_SUMMARY.md)

---

**Happy coding!** 🚀

*Last updated: 2025-10-14*

# llm-manager

A CLI tool for managing LLM (Large Language Model) resources and configurations.

## Features

- **Version management** — Track and manage different LLM model versions
- **Configuration management** — Centralized configuration with environment variable overrides
- **Cross-platform** — Works on Linux, macOS, and Windows
- **Extensible** — Modular architecture for adding new commands

## Installation

### From source

```bash
go install github.com/user/llm-manager/cmd/llm-manager@latest
```

### From source (development)

```bash
git clone https://github.com/user/llm-manager.git
cd llm-manager
make build
./bin/llm-manager
```

## Usage

```bash
# Show version
llm-manager version

# Show help
llm-manager help

# Show configuration
llm-manager config

# Short version
llm-manager -V
```

## Commands

| Command      | Description                    |
|-------------|--------------------------------|
| `help`      | Show this help message         |
| `version`   | Show version information       |
| `config`    | Show current configuration     |

## Environment Variables

| Variable             | Description                          | Default                  |
|---------------------|--------------------------------------|--------------------------|
| `LLM_MANAGER_VERBOSE` | Enable verbose output (`true`/`1`)  | `false`                  |
| `LLM_MANAGER_CONFIG`  | Path to configuration file          | `~/.config/llm-manager/` |
| `LLM_MANAGER_DATA_DIR`| Path to data directory              | `~/.local/share/llm-manager` |
| `LLM_MANAGER_LOG_DIR` | Path to log directory               | `~/.local/log/llm-manager` |

## Build

```bash
# Build with version info
make build VERSION=v1.0.0 COMMIT=$(git rev-parse --short HEAD) DATE=$(date -u +%Y-%m-%dT%H:%M:%SZ)

# Run tests
make test

# Run linter
make lint

# Format code
make fmt

# Clean build artifacts
make clean

# Full verification (fmt + lint + test)
make verify
```

## Project Structure

```
llm-manager/
├── cmd/
│   └── llm-manager/       # CLI entry point
│       └── main.go
├── internal/              # Private packages
│   ├── cmd/               # Command implementations
│   │   └── root.go
│   ├── config/            # Configuration management
│   │   └── config.go
│   └── version/           # Version information
│       └── version.go
├── pkg/                   # Public packages
│   └── version/           # Public version API
│       └── version.go
├── docs/                  # Documentation
├── test/                  # Test fixtures
├── Makefile               # Build automation
├── go.mod                 # Go module definition
└── README.md              # This file
```

## Development

### Prerequisites

- Go 1.21 or later
- Make (optional, for build automation)

### Setting up the development environment

```bash
# Install development dependencies
go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest

# Run all checks
make verify
```

### Adding a new command

1. Create a new file in `internal/cmd/` (e.g., `internal/cmd/newcmd.go`)
2. Implement the command logic
3. Add the command handler in `internal/cmd/root.go`
4. Update the help message

## License

MIT

# Architecture

## Overview

llm-manager follows a clean, modular architecture designed for CLI applications in Go.

## Directory Layout

### `cmd/llm-manager/`

Contains the main package entry point. This directory should only contain the `main()` function and minimal argument parsing that dispatches to internal packages.

### `internal/`

Private packages that are not imported by external code.

- **`cmd/`** — Command implementations and CLI logic
- **`config/`** — Configuration loading and management
- **`version/`** — Version information and build metadata

### `pkg/`

Public packages that may be imported by external code.

- **`version/`** — Public API for version information

### `docs/`

Documentation files.

### `test/`

Test fixtures and integration tests.

## Design Principles

1. **Internal packages** — Business logic lives in `internal/` and cannot be imported by external projects
2. **Public API** — Only `pkg/` packages are exposed for external consumption
3. **Dependency injection** — Commands receive dependencies via interfaces
4. **Configuration** — Centralized config with environment variable overrides
5. **Version injection** — Build-time version info via ldflags

## Command Pattern

Commands follow a simple pattern:

```go
type Command struct {
    cfg *config.Config
}

func (c *Command) Run(args []string) int {
    // implementation
    return exitCode
}
```

Each command returns an exit code:
- `0` — Success
- `1` — Error

## Version Management

Version information is injected at build time via ldflags:

```bash
go build -ldflags "-X github.com/user/llm-manager/internal/version.version=v1.0.0 \
                   -X github.com/user/llm-manager/internal/version.commit=abc123 \
                   -X github.com/user/llm-manager/internal/version.date=2024-01-01T00:00:00Z" \
    -o bin/llm-manager ./cmd/llm-manager
```

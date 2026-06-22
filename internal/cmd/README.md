# Command Package

All CLI subcommands for llm-manager. Follows the single-struct-per-command pattern: one struct, one `Run()` method, one `init()` registration function per file.

## Target Language

**Go 1.26.2** — Check Context7 MCP before making decisions about stdlib packages, generics, logging patterns, or error handling approaches.

## Shared Infrastructure

Read these first — everything depends on them:

| File          | Responsibility                                                                 |
|---------------|--------------------------------------------------------------------------------|
| `root.go`     | `RootCommand` struct — shared config pointer, DB connection (`*gorm.DB`), global flag parsing (`--api-port`, `--api-host`) |
| `registry.go` | Command registry — `RegisterCommand() + lookup` powering `main.go`'s subcommand routing |
| `sigint.go`   | `RunInteractive` — signal-safe subprocess runner (forwards SIGINT to child processes) |

## Core Container Management

| File                 | Subcommand           | Responsibility                                                        |
|----------------------|---------------------|-----------------------------------------------------------------------|
| `llm.go`             | `llm <sub>`          | Full LLM container lifecycle: start, stop, restart, swap, status, logs streaming |
| `container_swap.go`  | `swap <slug>`        | Container swap: stop old → pull/push tag → start new                   |
| `service.go`         | `service <sub>`      | High-level orchestration: list/start/stop/restart (delegates to llm)   |
| `config.go`          | `config <sub>`       | Persistent key-value store with secret encryption support              |

## Model & Engine Operations

| File                 | Subcommand              | Responsibility                                                     |
|----------------------|------------------------|--------------------------------------------------------------------|
| `model.go`           | `model <sub>`           | Model CRUD + cache management + pretty-print helpers                |
| `engine.go`          | `engine <sub>`          | Engine type/version management                                     |
| `import.go`          | `import <path>`         | Import models/engine definitions from YAML                           |
| `export.go`          | `export <path>`         | Export models/engine definitions to YAML                             |
| `install.go`         | `install <slug>`        | Install model weights into correct directory structure              |

## Container-Specific Commands

| File                 | Subcommand           | Responsibility                                                    |
|----------------------|---------------------|-------------------------------------------------------------------|
| `comfyui.go`         | `comfyui <sub>`      | ComfyUI container: start, stop, Flux workflow management            |
| `speech.go`          | `speech <sub>`         | Speech models: STT, TTS, Omni lifecycle                              |
| `update.go`          | `update <slug>`      | HF weight pull/update operations                                    |
| `rag.go`             | `rag <sub>`            | RAG model operations                                                |
| `hotspot.go`         | `hotspot <sub>`        | GPU hotspot reporting commands                                       |
| `logs.go`            | `logs <sub>`           | Log streaming with formatting                                        |
| `mem.go`             | `mem <sub>`            | Memory utilization reporting                                          |
| `uninstall.go`       | `uninstall`            | Full uninstall/cleanup                                                |
| `generate_opencode.go` | `generate-opencode`  | OpenCode config generation from local models                         |

## Deprecated / Stub Files

These files exist only to avoid import errors. They contain no implementation:

| File               | Status                                  |
|--------------------|-----------------------------------------|
| `embed.go`         | DEPRECATED — stub, no-op package        |
| `rerank.go`        | DEPRECATED — stub, no-op package        |
| `engine_import.go` | DEPRECATED — removed after split into import.go/en gine.go/service/ |

## Adding a New Command — Template

Copy any existing command file and adapt:

```go
func init() {
    RegisterCommand("mycommand", func(root *RootCommand) Command {
        return NewMyCommand(root)
    })
}

type MyCommand struct {
    cfg *RootCommand
    svc *service.SomeService
}

func NewMyCommand(root *RootCommand) *MyCommand {
    return &MyCommand{cfg: root, svc: service.NewSomeService(root.db)}
}

func (c *MyCommand) Run(args []string) int {
    switch len(args) == 0 {
    default: c.PrintHelp(); return 0 }
    switch args[0] {
    case "list": return c.runList()
    case "help", "-h": c.PrintHelp(); return 0 }
}

func (c *MyCommand) runList() int { /* ... */ }
func (c *MyCommand) PrintHelp()   { /* help text */ }
```

## Testing

```bash
# Run all CMD tests
go test ./internal/cmd/... -count=1

# Run specific integration tests
go test ./internal/cmd/... -run TestImport -v
```

Test files follow `*_test.go` naming convention. Integration tests use temp directories. Unit tests mock the service layer where available.

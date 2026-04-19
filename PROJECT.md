# llm-manager

Go CLI for managing LLM containers and services.

## Decisions & Notes

- [2026-04-17] Architecture: Established service layer (`internal/service/`) + 7 subcommand packages (`model`, `container`, `service`, `hotspot`, `logs`, `update`, `mem`) extending CLI from 4 to 11 commands. Reason: separate business logic from CLI concerns, improve testability.
- [2026-04-17] Database: RootCommand owns a single `*gorm.DB` connection shared by all subcommands. Reason: prevents connection leaks, ensures consistent error handling. Departure from original pattern where each command opened its own connection.
- [2026-04-17] The `DatabaseManager` interface in `internal/database/manager.go` already had all CRUD methods defined — they were implemented in `sqlite.go` but not wired into CLI commands. The `RootCommand` struct needed a `db` field added (was only `cfg` before) to expose this to the service layer.

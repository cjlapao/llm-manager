# Learnings

## Top-Level Patterns

| Category | Lesson | Prevent |
|---|---|---|
| DB Lifecycle | RootCommand opens DB once in `Run()`, stores on `db` field, defers close. All subcommands share this connection. | Having each command create its own DB connection → connection leaks and inconsistent error handling. |
| Dependency Injection | Subcommands receive dependencies via the `RootCommand` reference rather than constructing them internally. | Commands reaching directly into global state or re-initializing shared resources. |
| Service Layer | Thin business logic layer (`internal/service/`) wraps the database. Each command gets its own service dependency. | Commands containing raw SQL or direct DB calls, making them hard to test and reuse. |

## Raw Activity Log

- [2026-04-17] CLI extensible from 4 to 11 commands by adding service layer + 7 subcommand packages. RootCommand `db` field was the missing piece wiring `DatabaseManager` CRUD methods (already implemented in `sqlite.go`) into CLI commands.
- [2026-04-17] Service layer tests use `:memory:` SQLite with `glebarez/sqlite` driver — confirmed working for unit tests.

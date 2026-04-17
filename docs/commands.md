# Commands

## `version`

Display version information for llm-manager.

### Usage

```bash
llm-manager version
```

### Output

```
llm-manager version v1.0.0 (commit: abc1234) built at 2024-01-01T00:00:00Z
go version: go1.21.0
architecture: linux/amd64
```

### Flags

| Flag | Description |
|------|-------------|
| `-v` | Short version output |
| `--version` | Full version output |

---

## `config`

Display the current configuration.

### Usage

```bash
llm-manager config
```

### Output

```
llm-manager config:
  verbose:     false
  config file:
  home dir:    /home/user
  data dir:    /home/user/.local/share/llm-manager
  log dir:     /home/user/.local/log/llm-manager
```

---

## `help`

Display help information.

### Usage

```bash
llm-manager help
llm-manager --help
llm-manager -h
```

# MySQL Health Check

A lightweight MySQL health check tool for Debian 12. Connects to MySQL using a `.my.cnf` file and runs checks across system metrics, storage engines, memory, and query performance.

## Installation

### From GitHub Release

```bash
# Download (replace VERSION with the release tag, e.g. v1.0.0)
wget https://github.com/hypernode/mysql-health-check/releases/download/VERSION/mysql-health-check-linux-amd64

# Make executable
chmod +x mysql-health-check-linux-amd64

# Optional: rename
mv mysql-health-check-linux-amd64 mysql-health-check
```

### Build from source

```bash
go build -o mysql-health-check .
```

## Usage

```bash
mysql-health-check [options]
```

### Options

| Flag | Default | Description |
|------|---------|-------------|
| `-cnf` | `/data/web/.my.cnf` | Path to `.my.cnf` credentials file |
| `-sample-seconds` | `3` | CPU sample duration in seconds |
| `-no-color` | `false` | Disable ANSI color output |
| `-version` | - | Show version and exit |

### Example

```bash
# Use default config location
./mysql-health-check

# Specify custom .my.cnf
./mysql-health-check -cnf /etc/mysql/.my.cnf

# Disable colors (e.g. for logs)
./mysql-health-check -no-color

# Longer CPU sampling
./mysql-health-check -sample-seconds 5
```

## Exit codes

| Code | Meaning |
|------|---------|
| 0 | All checks OK |
| 1 | Warning(s) |
| 2 | Critical error(s) or connection failure |

## Requirements

- Debian 12 (warnings shown on other OS)
- MySQL/MariaDB with `.my.cnf` containing `[client]` with `user` and `password`

## Checks performed

- **System** — CPU, connections, threads
- **MyISAM / InnoDB** — Engine status and buffers
- **Memory** — Cache hit ratios, buffer pool
- **Queries / Logs** — Slow query log, binlog

# MySQL Health Check

A lightweight MySQL health check tool for Debian 12. Connects to MySQL using a `.my.cnf` file and runs checks across system metrics, storage engines, memory, and query performance.

## Installation

### From GitHub Release

```bash
# Download (replace VERSION with the release tag, e.g. v1.0.0)
# Linux AMD64
wget https://github.com/hpowernl/MySQL_check/releases/download/VERSION/mysql-health-check-linux-amd64

# Linux ARM64
wget https://github.com/hpowernl/MySQL_check/releases/download/VERSION/mysql-health-check-linux-arm64

# Make executable
chmod +x mysql-health-check-linux-amd64

# Optional: rename
mv mysql-health-check-linux-amd64 mysql-health-check
```

### Verify Checksums

```bash
wget https://github.com/hpowernl/MySQL_check/releases/download/VERSION/checksums.txt
sha256sum -c checksums.txt
```

### Build from Source

Requires [Go 1.22+](https://go.dev/dl/).

```bash
go build -o mysql-health-check .

# With version (for releases)
go build -ldflags="-X main.Version=v1.0.0" -o mysql-health-check .
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

### Examples

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

## Exit Codes

| Code | Meaning |
|------|---------|
| 0 | All checks OK |
| 1 | Warning(s) |
| 2 | Critical error(s) or connection failure |

## Requirements

- Debian 12 (warnings shown on other OS)
- MySQL/MariaDB with `.my.cnf` containing `[client]` with `user` and `password`

## Checks Performed

### System
- **CPU Utilization** — mysqld process CPU usage (≤80% OK, 80–100% WARN, >100% CRIT)
- **Disk Space Usage** — Data directory filesystem usage (<80% OK, ≥80% WARN)
- **Memory Utilization** — Server RAM usage (<80% OK, ≥80% WARN)
- **Connection Utilization** — Peak usage of max_connections (<70% OK, 70–85% WARN, ≥85% CRIT)
- **Open Files Utilization** — File descriptor usage (<85% OK, ≥85% WARN; SKIP on MySQL 8.0+ where this counter is not tracked)

### MyISAM / InnoDB
- **MyISAM Cache Hit Rate** — Key buffer effectiveness (>95% OK, ≤95% WARN)
- **MyISAM Key Write Ratio** — Physical key block write efficiency (≥90% OK)
- **InnoDB Cache Hit Rate** — Buffer pool hit rate; high physical reads indicate `innodb_buffer_pool_size` is too small (>90% OK, ≤90% WARN)
- **InnoDB Buffer Pool Wait Free** — Hard evidence of buffer pool pressure; any stall waiting for a free page means the pool is undersized (0 OK, >0 WARN)
- **InnoDB Log File Size** — Redo log coverage in minutes; too short causes checkpoint I/O spikes, too large slows crash recovery (45–120min OK, outside range WARN)
- **InnoDB Dirty Pages Ratio** — Modified pages not yet flushed; high ratio signals flushing cannot keep up (<75% OK, ≥75% WARN)
- **InnoDB Pending I/O** — Pending write and fsync operations; structurally elevated values indicate `innodb_io_capacity` is too low (0/0 OK, either >0 WARN)

### Memory
- **Thread Cache Hit Rate** — Thread reuse efficiency (>50% OK)
- **Thread Cache Ratio** — Cached vs created threads (>10% OK)
- **Table Cache Hit Rate** — Table open cache effectiveness (≥90% OK)
- **Table Def Cache Hit Rate** — Table definition cache hit rate (>75% OK)
- **Table Cache Overflows** — Times a table handle was evicted due to a full cache; any overflow means `table_open_cache` is too small (0 OK, >0 WARN)
- **Table Locking Efficiency** — Locks acquired without waiting (>95% OK)

### Queries / Logs
- **Sort Merge Passes Ratio** — Sort operations spilling to disk (<10% OK)
- **Sort Buffer Memory Risk** — Worst-case peak memory of `sort_buffer_size × max_connections` relative to total RAM; warns before a concurrency spike causes memory exhaustion (<25% OK, ≥25% WARN)
- **Temporary Disk Data** — Temp tables created on disk (≤25% OK, >25% WARN)
- **Flushing Logs** — Log buffer flush waits (<5% OK, 5–20% WARN, >20% CRIT)
- **QCache Fragmentation** — Query cache fragmentation (MySQL <8.0 only)
- **Query Truncation Status** — Truncated SQL in performance_schema

## Development

```bash
go vet ./...
go test -v ./...
```

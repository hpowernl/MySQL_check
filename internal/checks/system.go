package checks

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/hypernode/mysql-health-check/internal/db"
)

func RunSystemChecks(m *db.MySQL, sampleSeconds int) []Check {
	var results []Check
	results = append(results, checkCPU(sampleSeconds))
	results = append(results, checkDiskSpace(m))
	results = append(results, checkMemory())
	results = append(results, checkConnectionUtilization(m))
	results = append(results, checkOpenFiles(m))
	return results
}

func checkCPU(sampleSeconds int) Check {
	c := Check{
		Name:      "CPU Utilization",
		Threshold: "<= 80% OK, 80-100% WARN, > 100% CRIT",
		Description: "Average CPU usage by the mysqld process.",
		Detail: "CPU utilization measures how much processing power mysqld is consuming " +
			"relative to the available cores. High sustained CPU usage (above 80%) may " +
			"indicate poorly optimized queries, missing indexes, or that the server needs " +
			"more processing capacity. Values above 100% indicate contention across cores.",
	}

	pid, err := findMysqldPid()
	if err != nil {
		c.Value = "N/A"
		c.Level = LevelSkip
		return c
	}

	statPath := fmt.Sprintf("/proc/%d/stat", pid)
	t1, err := readProcCPUTicks(statPath)
	if err != nil {
		c.Value = "N/A"
		c.Level = LevelSkip
		return c
	}

	time.Sleep(time.Duration(sampleSeconds) * time.Second)

	t2, err := readProcCPUTicks(statPath)
	if err != nil {
		c.Value = "N/A"
		c.Level = LevelSkip
		return c
	}

	hz := sysconfCLKTCK()
	delta := float64(t2-t1) / float64(hz)
	cpuCount := numCPU()
	usage := (delta / float64(sampleSeconds)) * 100.0 / float64(cpuCount)

	c.Value = fmtPct(usage)
	switch {
	case usage <= 80:
		c.Level = LevelOK
	case usage <= 100:
		c.Level = LevelWarn
	default:
		c.Level = LevelCrit
	}
	return c
}

func checkDiskSpace(m *db.MySQL) Check {
	c := Check{
		Name:      "Disk Space Usage",
		Threshold: "< 80% OK, >= 80% WARN",
		Description: "Percentage of used disk space on the MySQL data directory filesystem.",
		Detail: "Monitors the filesystem where MySQL stores its data files. Running out of " +
			"disk space can cause MySQL to crash, corrupt data, or refuse writes entirely. " +
			"Proactive monitoring prevents catastrophic failures. Keep at least 20% free for " +
			"operations like ALTER TABLE, binary logs, and temporary files.",
	}

	datadir, ok := m.Vars["datadir"]
	if !ok || datadir == "" {
		c.Value = "N/A"
		c.Level = LevelSkip
		return c
	}

	var stat syscall.Statfs_t
	if err := syscall.Statfs(datadir, &stat); err != nil {
		c.Value = "N/A"
		c.Level = LevelSkip
		return c
	}

	total := stat.Blocks * uint64(stat.Bsize)
	free := stat.Bfree * uint64(stat.Bsize)
	if total == 0 {
		c.Value = "N/A"
		c.Level = LevelSkip
		return c
	}
	used := total - free
	usage := float64(used) * 100.0 / float64(total)

	c.Value = fmtPct(usage)
	if usage < 80 {
		c.Level = LevelOK
	} else {
		c.Level = LevelWarn
	}
	return c
}

func checkMemory() Check {
	c := Check{
		Name:      "Memory Utilization",
		Threshold: "< 80% OK, >= 80% WARN",
		Description: "Current memory usage of the server.",
		Detail: "Measures how much of the server's physical RAM is in use. MySQL relies " +
			"heavily on memory for the InnoDB buffer pool, thread stacks, sort buffers, and " +
			"caches. If memory utilization consistently exceeds 80%, the server may start " +
			"swapping to disk, which drastically reduces database performance.",
	}

	memTotal, memAvail, err := readMeminfo()
	if err != nil {
		c.Value = "N/A"
		c.Level = LevelSkip
		return c
	}

	if memTotal == 0 {
		c.Value = "N/A"
		c.Level = LevelSkip
		return c
	}
	used := memTotal - memAvail
	usage := float64(used) * 100.0 / float64(memTotal)

	c.Value = fmtPct(usage)
	if usage < 80 {
		c.Level = LevelOK
	} else {
		c.Level = LevelWarn
	}
	return c
}

func checkConnectionUtilization(m *db.MySQL) Check {
	c := Check{
		Name:      "Connection Utilization",
		Threshold: "< 70% OK, 70-85% WARN, >= 85% CRIT",
		Description: "Utilization of available database connections.",
		Detail: "Shows the peak percentage of max_connections that has been used since the " +
			"server started. If this approaches 85-100%, new connections may be refused, " +
			"causing application errors. If consistently high, consider increasing " +
			"max_connections or investigating connection pooling.",
	}

	maxUsed := statusFloat(m, "Max_used_connections")
	maxConn := varFloat(m, "max_connections")
	v, ok := pct(maxUsed, maxConn)
	if !ok {
		c.Value = "N/A"
		c.Level = LevelSkip
		return c
	}

	c.Value = fmtPct(v)
	switch {
	case v < 70:
		c.Level = LevelOK
	case v < 85:
		c.Level = LevelWarn
	default:
		c.Level = LevelCrit
	}
	return c
}

func checkOpenFiles(m *db.MySQL) Check {
	c := Check{
		Name:      "Open Files Utilization",
		Threshold: "< 85% OK, >= 85% WARN",
		Description: "Usage of file descriptors by MySQL.",
		Detail: "MySQL opens file descriptors for table data files, log files, and " +
			"connections. If the open files count approaches the OS limit, MySQL cannot " +
			"open new tables or accept new connections, leading to errors. Ensure the " +
			"open_files_limit is high enough for your workload.",
	}

	openFiles := statusFloat(m, "Open_files")
	limit := varFloat(m, "open_files_limit")
	v, ok := pct(openFiles, limit)
	if !ok {
		c.Value = "N/A"
		c.Level = LevelSkip
		return c
	}

	c.Value = fmtPct(v)
	if v < 85 {
		c.Level = LevelOK
	} else {
		c.Level = LevelWarn
	}
	return c
}

// --- helpers ---

func statusFloat(m *db.MySQL, key string) float64 {
	v, ok := m.Status[key]
	if !ok {
		return 0
	}
	f, _ := strconv.ParseFloat(v, 64)
	return f
}

func varFloat(m *db.MySQL, key string) float64 {
	v, ok := m.Vars[key]
	if !ok {
		return 0
	}
	f, _ := strconv.ParseFloat(v, 64)
	return f
}

func findMysqldPid() (int, error) {
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return 0, err
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		pid, err := strconv.Atoi(e.Name())
		if err != nil {
			continue
		}
		comm, err := os.ReadFile(filepath.Join("/proc", e.Name(), "comm"))
		if err != nil {
			continue
		}
		if strings.TrimSpace(string(comm)) == "mysqld" {
			return pid, nil
		}
	}
	return 0, fmt.Errorf("mysqld process not found")
}

func readProcCPUTicks(path string) (int64, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return 0, err
	}
	fields := strings.Fields(string(data))
	if len(fields) < 15 {
		return 0, fmt.Errorf("unexpected /proc/pid/stat format")
	}
	utime, err := strconv.ParseInt(fields[13], 10, 64)
	if err != nil {
		return 0, err
	}
	stime, err := strconv.ParseInt(fields[14], 10, 64)
	if err != nil {
		return 0, err
	}
	return utime + stime, nil
}

func sysconfCLKTCK() int {
	return 100
}

func numCPU() int {
	data, err := os.ReadFile("/proc/cpuinfo")
	if err != nil {
		return 1
	}
	count := 0
	for _, line := range strings.Split(string(data), "\n") {
		if strings.HasPrefix(line, "processor") {
			count++
		}
	}
	if count == 0 {
		return 1
	}
	return count
}

func readMeminfo() (total, available uint64, err error) {
	f, err := os.Open("/proc/meminfo")
	if err != nil {
		return 0, 0, err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "MemTotal:") {
			total = parseKB(line)
		} else if strings.HasPrefix(line, "MemAvailable:") {
			available = parseKB(line)
		}
	}
	return total, available, scanner.Err()
}

func parseKB(line string) uint64 {
	fields := strings.Fields(line)
	if len(fields) < 2 {
		return 0
	}
	v, _ := strconv.ParseUint(fields[1], 10, 64)
	return v * 1024
}

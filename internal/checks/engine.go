package checks

import (
	"fmt"
	"strconv"

	"github.com/hypernode/mysql-health-check/internal/db"
)

func RunEngineChecks(m *db.MySQL) []Check {
	var results []Check
	results = append(results, checkMyISAMCacheHitRate(m))
	results = append(results, checkMyISAMKeyWriteRatio(m))
	results = append(results, checkInnoDBCacheHitRate(m))
	results = append(results, checkRedoLogCoverage(m))
	results = append(results, checkInnoDBDirtyPages(m))
	return results
}

func checkMyISAMCacheHitRate(m *db.MySQL) Check {
	c := Check{
		Name:      "MyISAM Cache Hit Rate",
		Threshold: "> 95% OK, <= 95% WARN",
		Description: "Effectiveness of the MyISAM key cache (index access).",
		Detail: "Measures what percentage of MyISAM index read requests are served from " +
			"the key buffer cache rather than from disk. A rate below 95% means MySQL " +
			"frequently reads index blocks from disk, which is significantly slower. " +
			"Increase key_buffer_size if this is low and you use MyISAM tables.",
	}

	reads := statusFloat(m, "Key_reads")
	requests := statusFloat(m, "Key_read_requests")
	if requests == 0 {
		c.Value = "N/A"
		c.Level = LevelSkip
		return c
	}

	v := 100.0 - (reads * 100.0 / requests)
	c.Value = fmtPct(v)
	if v > 95 {
		c.Level = LevelOK
	} else {
		c.Level = LevelWarn
	}
	return c
}

func checkMyISAMKeyWriteRatio(m *db.MySQL) Check {
	c := Check{
		Name:      "MyISAM Key Write Ratio",
		Threshold: "efficiency >= 90% OK, < 90% WARN",
		Description: "The proportion of physical writes of a key block to the cache.",
		Detail: "Shows what fraction of MyISAM key write requests result in actual " +
			"physical disk writes. A low ratio means most writes are absorbed by the " +
			"cache before being flushed to disk, which is ideal. High physical write " +
			"ratios indicate the key buffer is too small to effectively batch writes.",
	}

	writes := statusFloat(m, "Key_writes")
	requests := statusFloat(m, "Key_write_requests")
	if requests == 0 {
		c.Value = "N/A"
		c.Level = LevelSkip
		return c
	}

	ratio := writes * 100.0 / requests
	efficiency := 100.0 - ratio
	c.Value = fmt.Sprintf("%.2f%% (eff: %.2f%%)", ratio, efficiency)
	if efficiency >= 90 {
		c.Level = LevelOK
	} else {
		c.Level = LevelWarn
	}
	return c
}

func checkInnoDBCacheHitRate(m *db.MySQL) Check {
	c := Check{
		Name:      "InnoDB Cache Hit Rate",
		Threshold: "> 90% OK, <= 90% WARN",
		Description: "How often data is retrieved from the buffer pool instead of disk.",
		Detail: "The InnoDB buffer pool is the most critical memory structure in MySQL. " +
			"This metric shows the percentage of data page reads served from RAM. A hit " +
			"rate below 90% means MySQL is doing excessive disk I/O, which is orders of " +
			"magnitude slower. The primary fix is increasing innodb_buffer_pool_size.",
	}

	requests := statusFloat(m, "Innodb_buffer_pool_read_requests")
	reads := statusFloat(m, "Innodb_buffer_pool_reads")
	if requests == 0 {
		c.Value = "N/A"
		c.Level = LevelSkip
		return c
	}

	v := (requests - reads) * 100.0 / requests
	c.Value = fmtPct(v)
	if v > 90 {
		c.Level = LevelOK
	} else {
		c.Level = LevelWarn
	}
	return c
}

func checkRedoLogCoverage(m *db.MySQL) Check {
	c := Check{
		Name:      "InnoDB Log File Size",
		Threshold: ">= 45min OK, < 45min WARN (ideal 45-75min)",
		Description: "Minutes of redo log capacity before a flush is required.",
		Detail: "The InnoDB redo log records all changes to data. This check calculates " +
			"how many minutes of write activity the redo log can hold before it must be " +
			"flushed. Ideally this should be around 60 minutes (45-75 range). Too small " +
			"means frequent checkpoint flushes causing I/O spikes; too large means longer " +
			"crash recovery times.",
	}

	uptimeStr, ok := m.Status["Uptime"]
	if !ok {
		c.Value = "N/A"
		c.Level = LevelSkip
		return c
	}
	uptime, _ := strconv.ParseFloat(uptimeStr, 64)

	osLogWritten := statusFloat(m, "Innodb_os_log_written")
	if osLogWritten == 0 {
		c.Value = "N/A"
		c.Level = LevelSkip
		return c
	}

	var redoCap float64
	if m.VersionAtLeast(8, 0, 30) {
		if v, ok := m.Vars["innodb_redo_log_capacity"]; ok {
			redoCap, _ = strconv.ParseFloat(v, 64)
		}
	}
	if redoCap == 0 {
		filesInGroup := varFloat(m, "innodb_log_files_in_group")
		fileSize := varFloat(m, "innodb_log_file_size")
		redoCap = filesInGroup * fileSize
	}

	if redoCap == 0 {
		c.Value = "N/A"
		c.Level = LevelSkip
		return c
	}

	minutes := (uptime / 60.0) * redoCap / osLogWritten
	c.Value = fmtMin(minutes)
	if minutes >= 45 {
		c.Level = LevelOK
	} else {
		c.Level = LevelWarn
	}
	return c
}

func checkInnoDBDirtyPages(m *db.MySQL) Check {
	c := Check{
		Name:      "InnoDB Dirty Pages Ratio",
		Threshold: "< 75% OK, >= 75% WARN",
		Description: "Percentage of modified pages in memory not yet written back to disk.",
		Detail: "Dirty pages are data pages modified in the buffer pool but not yet flushed " +
			"to disk. A high ratio (>= 75%) during normal operations suggests the flushing " +
			"mechanism cannot keep up with writes, potentially leading to stalls when the " +
			"buffer pool runs out of clean pages. Tune innodb_io_capacity and " +
			"innodb_max_dirty_pages_pct.",
	}

	dirty := statusFloat(m, "Innodb_buffer_pool_pages_dirty")
	total := statusFloat(m, "Innodb_buffer_pool_pages_total")
	v, ok := pct(dirty, total)
	if !ok {
		c.Value = "N/A"
		c.Level = LevelSkip
		return c
	}

	c.Value = fmtPct(v)
	if v < 75 {
		c.Level = LevelOK
	} else {
		c.Level = LevelWarn
	}
	return c
}

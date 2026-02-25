package checks

import (
	"fmt"
	"strconv"

	"github.com/hpowernl/MySQL_check/internal/db"
)

func RunQueryChecks(m *db.MySQL) []Check {
	var results []Check
	results = append(results, checkSortMergePassRatio(m))
	results = append(results, checkTempDiskData(m))
	results = append(results, checkFlushingLogs(m))
	results = append(results, checkQCacheFragmentation(m))
	results = append(results, checkQueryTruncation(m))
	return results
}

func checkSortMergePassRatio(m *db.MySQL) Check {
	c := Check{
		Name:      "Sort Merge Passes Ratio",
		Threshold: "< 10% OK, >= 10% WARN",
		Description: "The effectiveness of sorting operations.",
		Detail: "When MySQL cannot complete a sort in memory, it writes temporary data " +
			"to disk and performs merge passes. A high ratio means many sorts spill to " +
			"disk, significantly slowing query execution. Increase sort_buffer_size to " +
			"allow more sorts to complete in memory, and optimize queries to reduce the " +
			"amount of data sorted.",
	}

	passes := statusFloat(m, "Sort_merge_passes")
	scans := statusFloat(m, "Sort_scan")
	ranges := statusFloat(m, "Sort_range")
	denom := scans + ranges
	v, ok := pct(passes, denom)
	if !ok {
		c.Value = "N/A"
		c.Level = LevelSkip
		return c
	}

	c.Value = fmtPct(v)
	if v < 10 {
		c.Level = LevelOK
	} else {
		c.Level = LevelWarn
	}
	return c
}

func checkTempDiskData(m *db.MySQL) Check {
	c := Check{
		Name:      "Temporary Disk Data",
		Threshold: "<= 25% OK, > 25% WARN",
		Description: "The percentage of temporary tables created on disk instead of in memory.",
		Detail: "MySQL creates temporary tables for complex queries (GROUP BY, DISTINCT, " +
			"UNION). When these exceed tmp_table_size or max_heap_table_size, they spill " +
			"to disk. A ratio above 25% indicates significant disk-based temp table usage. " +
			"Increase tmp_table_size and max_heap_table_size, and optimize queries to " +
			"reduce temporary table sizes.",
	}

	diskTables := statusFloat(m, "Created_tmp_disk_tables")
	totalTables := statusFloat(m, "Created_tmp_tables")
	v, ok := pct(diskTables, totalTables)
	if !ok {
		c.Value = "N/A"
		c.Level = LevelSkip
		return c
	}

	c.Value = fmtPct(v)
	if v <= 25 {
		c.Level = LevelOK
	} else {
		c.Level = LevelWarn
	}
	return c
}

func checkFlushingLogs(m *db.MySQL) Check {
	c := Check{
		Name:      "Flushing Logs",
		Threshold: "< 5% OK, 5-20% WARN, > 20% CRIT",
		Description: "The percentage of log writes that had to wait for the log buffer to be flushed.",
		Detail: "When InnoDB needs to write to the redo log but the log buffer is full, " +
			"it must wait for the buffer to be flushed to disk. A high wait percentage " +
			"means the innodb_log_buffer_size is too small for the write workload, causing " +
			"write stalls. Values above 20% require immediate attention to prevent " +
			"performance degradation.",
	}

	waits := statusFloat(m, "Innodb_log_waits")
	writes := statusFloat(m, "Innodb_log_writes")
	v, ok := pct(waits, writes)
	if !ok {
		c.Value = "N/A"
		c.Level = LevelSkip
		return c
	}

	c.Value = fmtPct(v)
	switch {
	case v < 5:
		c.Level = LevelOK
	case v <= 20:
		c.Level = LevelWarn
	default:
		c.Level = LevelCrit
	}
	return c
}

func checkQCacheFragmentation(m *db.MySQL) Check {
	c := Check{
		Name:      "QCache Fragmentation",
		Threshold: "frag < 10% AND del < 20% OK, else WARN",
		Description: "Query cache fragmentation and eviction rate.",
		Detail: "The query cache stores SELECT results for reuse. Fragmentation means " +
			"free memory is scattered in small blocks, reducing cache efficiency. A high " +
			"delete rate means queries are being evicted due to low memory before they " +
			"can be reused. Note: Query Cache is removed in MySQL 8.0+, so this check " +
			"only applies to older versions.",
	}

	freeBlocks, hasFree := m.Status["Qcache_free_blocks"]
	totalBlocks, hasTotal := m.Status["Qcache_total_blocks"]
	lowmemPrunes, hasPrunes := m.Status["Qcache_lowmem_prunes"]
	inserts, hasInserts := m.Status["Qcache_inserts"]

	if !hasFree || !hasTotal || !hasPrunes || !hasInserts {
		c.Value = "N/A"
		c.Level = LevelSkip
		return c
	}

	freeF, _ := strconv.ParseFloat(freeBlocks, 64)
	totalF, _ := strconv.ParseFloat(totalBlocks, 64)
	prunesF, _ := strconv.ParseFloat(lowmemPrunes, 64)
	insertsF, _ := strconv.ParseFloat(inserts, 64)

	frag, fragOK := pct(freeF, totalF)
	delRate := float64(0)
	delRateOK := false
	if insertsF > 0 {
		delRate = prunesF * 100.0 / insertsF
		delRateOK = true
	}

	if !fragOK {
		c.Value = "N/A"
		c.Level = LevelSkip
		return c
	}

	c.Value = fmt.Sprintf("frag=%.2f%% del=%.2f%%", frag, delRate)
	if frag < 10 && (!delRateOK || delRate < 20) {
		c.Level = LevelOK
	} else {
		c.Level = LevelWarn
	}
	return c
}

func checkQueryTruncation(m *db.MySQL) Check {
	c := Check{
		Name:      "Query Truncation Status",
		Threshold: "FALSE = OK, TRUE = WARN",
		Description: "The presence of truncated SQL query statements.",
		Detail: "When SQL query text exceeds the performance_schema max length, it gets " +
			"truncated with '...'. This prevents full analysis of slow or problematic " +
			"queries. If truncation is detected, incrementally increase max_digest_length, " +
			"performance_schema_max_sql_text_length, and " +
			"performance_schema_max_digest_length to capture complete query text.",
	}

	val, err := m.QueryScalar(
		"SELECT COUNT(*) FROM performance_schema.events_statements_history WHERE SQL_TEXT LIKE '%...'",
	)
	if err != nil {
		c.Value = "N/A"
		c.Level = LevelSkip
		return c
	}

	count, _ := strconv.Atoi(val)
	if count > 0 {
		c.Value = fmt.Sprintf("TRUE (%d truncated)", count)
		c.Level = LevelWarn
	} else {
		c.Value = "FALSE"
		c.Level = LevelOK
	}
	return c
}

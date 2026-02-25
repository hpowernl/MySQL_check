package checks

import (
	"github.com/hypernode/mysql-health-check/internal/db"
)

func RunCacheChecks(m *db.MySQL) []Check {
	var results []Check
	results = append(results, checkThreadCacheHitRate(m))
	results = append(results, checkThreadCacheRatio(m))
	results = append(results, checkTableCacheHitRate(m))
	results = append(results, checkTableDefCacheHitRate(m))
	results = append(results, checkTableLockingEfficiency(m))
	return results
}

func checkThreadCacheHitRate(m *db.MySQL) Check {
	c := Check{
		Name:      "Thread Cache Hit Rate",
		Threshold: "> 50% OK, <= 50% WARN",
		Description: "The percentage of times a requested thread is found in the cache.",
		Detail: "When a client connects, MySQL can reuse a cached thread instead of " +
			"creating a new one. Thread creation is expensive (involves memory allocation " +
			"and OS thread setup). A hit rate below 50% means more than half of connections " +
			"require new thread creation. Increase thread_cache_size to improve this.",
	}

	created := statusFloat(m, "Threads_created")
	connections := statusFloat(m, "Connections")
	if connections == 0 {
		c.Value = "N/A"
		c.Level = LevelSkip
		return c
	}

	v := 100.0 - (created * 100.0 / connections)
	c.Value = fmtPct(v)
	if v > 50 {
		c.Level = LevelOK
	} else {
		c.Level = LevelWarn
	}
	return c
}

func checkThreadCacheRatio(m *db.MySQL) Check {
	c := Check{
		Name:      "Thread Cache Ratio",
		Threshold: "> 10% OK, <= 10% WARN",
		Description: "The efficiency of the thread cache for reusing threads.",
		Detail: "Shows what proportion of all threads ever created are currently sitting " +
			"in the cache ready for reuse. A ratio below 10% suggests the thread cache is " +
			"undersized relative to the connection pattern. Increasing thread_cache_size " +
			"allows MySQL to keep more idle threads ready, reducing connection latency.",
	}

	cached := statusFloat(m, "Threads_cached")
	created := statusFloat(m, "Threads_created")
	v, ok := pct(cached, created)
	if !ok {
		c.Value = "N/A"
		c.Level = LevelSkip
		return c
	}

	c.Value = fmtPct(v)
	if v > 10 {
		c.Level = LevelOK
	} else {
		c.Level = LevelWarn
	}
	return c
}

func checkTableCacheHitRate(m *db.MySQL) Check {
	c := Check{
		Name:      "Table Cache Hit Rate",
		Threshold: ">= 90% OK, < 90% WARN",
		Description: "Efficiency of the table open cache.",
		Detail: "Each time MySQL accesses a table, it needs an open file handle. The " +
			"table cache stores these handles to avoid repeatedly opening and closing " +
			"files. A hit rate below 90% means MySQL frequently re-opens tables from " +
			"disk, adding latency. Increase table_open_cache if this is consistently low.",
	}

	if _, ok := m.Status["Table_open_cache_hits"]; ok {
		hitsF := statusFloat(m, "Table_open_cache_hits")
		missF := statusFloat(m, "Table_open_cache_misses")
		denom := hitsF + missF
		v, ok := pct(hitsF, denom)
		if !ok {
			c.Value = "N/A"
			c.Level = LevelSkip
			return c
		}
		c.Value = fmtPct(v)
		if v >= 90 {
			c.Level = LevelOK
		} else {
			c.Level = LevelWarn
		}
		return c
	}

	openTables := statusFloat(m, "Open_tables")
	openedTables := statusFloat(m, "Opened_tables")
	v, ok := pct(openTables, openedTables)
	if !ok {
		c.Value = "N/A"
		c.Level = LevelSkip
		return c
	}

	c.Value = fmtPct(v)
	if v >= 90 {
		c.Level = LevelOK
	} else {
		c.Level = LevelWarn
	}
	return c
}

func checkTableDefCacheHitRate(m *db.MySQL) Check {
	c := Check{
		Name:      "Table Def Cache Hit Rate",
		Threshold: "> 75% OK, <= 75% WARN",
		Description: "The efficiency of the table definition cache.",
		Detail: "Table definitions (schema metadata like column types, indexes) are " +
			"cached to avoid re-parsing .frm files or data dictionary entries. A hit " +
			"rate below 75% means MySQL frequently reloads table metadata, adding " +
			"overhead to every query. Increase table_definition_cache for databases " +
			"with many tables.",
	}

	openDefs := statusFloat(m, "Open_table_definitions")
	openedDefs := statusFloat(m, "Opened_table_definitions")
	v, ok := pct(openDefs, openedDefs)
	if !ok {
		c.Value = "N/A"
		c.Level = LevelSkip
		return c
	}

	c.Value = fmtPct(v)
	if v > 75 {
		c.Level = LevelOK
	} else {
		c.Level = LevelWarn
	}
	return c
}

func checkTableLockingEfficiency(m *db.MySQL) Check {
	c := Check{
		Name:      "Table Locking Efficiency",
		Threshold: "> 95% OK, <= 95% WARN",
		Description: "Percentage of table locks acquired without waiting.",
		Detail: "Measures how often table lock requests are granted immediately versus " +
			"having to wait. Low efficiency (< 95%) indicates lock contention, which " +
			"causes queries to queue and increases response times. If using MyISAM tables, " +
			"consider migrating to InnoDB which uses row-level locking instead of " +
			"table-level locking.",
	}

	immediate := statusFloat(m, "Table_locks_immediate")
	waited := statusFloat(m, "Table_locks_waited")
	denom := immediate + waited
	v, ok := pct(immediate, denom)
	if !ok {
		c.Value = "N/A"
		c.Level = LevelSkip
		return c
	}

	c.Value = fmtPct(v)
	if v > 95 {
		c.Level = LevelOK
	} else {
		c.Level = LevelWarn
	}
	return c
}

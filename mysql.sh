#!/usr/bin/env bash
set -euo pipefail

# Handy error context
trap 'echo "ERROR: line $LINENO: $BASH_COMMAND" >&2' ERR

CNF_DEFAULT="/data/web/.my.cnf"
CNF="$CNF_DEFAULT"
SAMPLE_SECONDS=3

usage() {
  cat <<'USAGE'
Usage:
  mysql-health-checks.sh [--cnf /path/to/.my.cnf] [--sample-seconds N]

Defaults:
  --cnf             /data/web/.my.cnf
  --sample-seconds  3
USAGE
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --cnf) CNF="${2:-}"; shift 2;;
    --sample-seconds) SAMPLE_SECONDS="${2:-3}"; shift 2;;
    -h|--help) usage; exit 0;;
    *) echo "Unknown arg: $1" >&2; usage; exit 2;;
  esac
done

if [[ ! -f "$CNF" ]]; then
  echo "ERROR: CNF file not found: $CNF" >&2
  exit 2
fi

MYSQL=(mysql --defaults-extra-file="$CNF" -N -B)

declare -A STATUS VAR

mysql_ok() { "${MYSQL[@]}" -e "SELECT 1" >/dev/null 2>&1; }
mysql_query() { "${MYSQL[@]}" -e "$1"; }

load_status_and_vars() {
  local out
  out="$(mysql_query "SHOW GLOBAL STATUS" 2>/dev/null || true)"
  while IFS=$'\t' read -r k v; do
    [[ -n "${k:-}" ]] && STATUS["$k"]="${v:-}"
  done <<< "$out"

  out="$(mysql_query "SHOW GLOBAL VARIABLES" 2>/dev/null || true)"
  while IFS=$'\t' read -r k v; do
    [[ -n "${k:-}" ]] && VAR["$k"]="${v:-}"
  done <<< "$out"
}

has_status() { [[ -n "${STATUS[$1]+_}" ]]; }
has_var()    { [[ -n "${VAR[$1]+_}" ]]; }

num() {
  local x="${1:-0}"
  [[ "$x" =~ ^[0-9]+([.][0-9]+)?$ ]] && echo "$x" || echo 0
}

pct() {
  local n d
  n="$(num "${1:-0}")"
  d="$(num "${2:-0}")"
  awk -v n="$n" -v d="$d" 'BEGIN{ if(d==0){print "NA"} else { printf "%.2f", (n*100.0)/d } }'
}

pct_sub_from_100() {
  local a b
  a="$(num "${1:-0}")"
  b="$(num "${2:-0}")"
  awk -v a="$a" -v b="$b" 'BEGIN{ if(b==0){print "NA"} else { printf "%.2f", 100.0 - (a*100.0/b) } }'
}

print_line() {
  printf "%-5s | %-30s | %-12s | %s\n" "$1" "$2" "$3" "$4"
}

# overall: 0=OK, 1=WARN, 2=CRIT
overall=0
bump_overall() {
  case "$1" in
    CRIT) overall=2 ;;
    WARN) [[ "$overall" -lt 1 ]] && overall=1 ;;
    *) : ;;
  esac
  return 0
}

mysqld_cpu_cores_sample() {
  local pid hz t1 t2 dticks secs
  pid="$(pgrep -xo mysqld 2>/dev/null || true)"
  [[ -z "$pid" ]] && { echo "NA"; return; }

  hz="$(getconf CLK_TCK 2>/dev/null || echo 100)"
  t1="$(awk '{print $14+$15}' "/proc/$pid/stat" 2>/dev/null || echo 0)"
  sleep "$SAMPLE_SECONDS"
  t2="$(awk '{print $14+$15}' "/proc/$pid/stat" 2>/dev/null || echo 0)"
  dticks=$(( t2 - t1 ))
  secs="$SAMPLE_SECONDS"

  awk -v dt="$dticks" -v hz="$hz" -v s="$secs" 'BEGIN{
    if(s<=0 || hz<=0){print "NA"; exit}
    printf "%.3f", (dt*1.0/hz)/s
  }'
}

if ! mysql_ok; then
  echo "ERROR: Unable to connect to MySQL using CNF: $CNF" >&2
  exit 2
fi

load_status_and_vars

MYSQL_VERSION="$(mysql_query "SELECT VERSION()" | head -n1)"
echo "MySQL version: $MYSQL_VERSION"
echo "CNF: $CNF"
echo

printf "%-5s | %-30s | %-12s | %s\n" "LVL" "CHECK" "VALUE" "NOTES"
printf -- "----- | ------------------------------ | ------------ | ------------------------------\n"

# 1 Thread Cache Hit Rate: 100 - Threads_created*100/Connections ; desired > 50
if has_status "Threads_created" && has_status "Connections"; then
  v="$(pct_sub_from_100 "${STATUS[Threads_created]}" "${STATUS[Connections]}")"
  lvl="INFO"; note="desired > 50"
  if [[ "$v" != "NA" ]]; then
    awk -v x="$v" 'BEGIN{exit (x>50)?0:1}' && lvl="OK" || lvl="WARN"
  fi
  print_line "$lvl" "1) Thread Cache Hit Rate" "$v%" "$note"
  bump_overall "$lvl"
else
  print_line "SKIP" "1) Thread Cache Hit Rate" "NA" "missing status vars"
fi

# 2 Thread Cache Ratio: Threads_cached*100/Threads_created ; threshold > 10
if has_status "Threads_cached" && has_status "Threads_created"; then
  v="$(pct "${STATUS[Threads_cached]}" "${STATUS[Threads_created]}")"
  lvl="INFO"; note="desired > 10"
  if [[ "$v" != "NA" ]]; then
    awk -v x="$v" 'BEGIN{exit (x>10)?0:1}' && lvl="OK" || lvl="WARN"
  fi
  print_line "$lvl" "2) Thread Cache Ratio" "$v%" "$note"
  bump_overall "$lvl"
else
  print_line "SKIP" "2) Thread Cache Ratio" "NA" "missing status vars"
fi

# 3 MyISAM Cache Hit Rate: 100 - Key_reads/Key_read_requests*100 ; good > 95
if has_status "Key_reads" && has_status "Key_read_requests"; then
  v="$(pct_sub_from_100 "${STATUS[Key_reads]}" "${STATUS[Key_read_requests]}")"
  lvl="INFO"; note="good > 95"
  if [[ "$v" != "NA" ]]; then
    awk -v x="$v" 'BEGIN{exit (x>95)?0:1}' && lvl="OK" || lvl="WARN"
  fi
  print_line "$lvl" "3) MyISAM Cache Hit Rate" "$v%" "$note"
  bump_overall "$lvl"
else
  print_line "SKIP" "3) MyISAM Cache Hit Rate" "NA" "missing status vars"
fi

# 4 MyISAM Key Write Efficiency: 100 - Key_writes*100/Key_write_requests ; desired >= 90
if has_status "Key_writes" && has_status "Key_write_requests"; then
  ratio_v="$(pct "${STATUS[Key_writes]}" "${STATUS[Key_write_requests]}")"
  eff_v="$(awk -v r="$ratio_v" 'BEGIN{ if(r=="NA"){print "NA"} else { printf "%.2f", 100.0-r } }')"
  lvl="INFO"; note="efficiency desired >= 90"
  if [[ "$eff_v" != "NA" ]]; then
    awk -v x="$eff_v" 'BEGIN{exit (x>=90)?0:1}' && lvl="OK" || lvl="WARN"
  fi
  print_line "$lvl" "4) MyISAM Key Write Efficiency" "$eff_v%" "ratio=${ratio_v}% ; $note"
  bump_overall "$lvl"
else
  print_line "SKIP" "4) MyISAM Key Write Efficiency" "NA" "missing status vars (InnoDB-only systems often skip)"
fi

# 5 InnoDB Cache Hit Rate: (read_requests - reads)*100/read_requests ; ideal > 90
if has_status "Innodb_buffer_pool_read_requests" && has_status "Innodb_buffer_pool_reads"; then
  rr="$(num "${STATUS[Innodb_buffer_pool_read_requests]}")"
  r="$(num "${STATUS[Innodb_buffer_pool_reads]}")"
  v="$(awk -v rr="$rr" -v r="$r" 'BEGIN{ if(rr==0){print "NA"} else { printf "%.2f", ((rr-r)*100.0)/rr } }')"
  lvl="INFO"; note="ideal > 90"
  if [[ "$v" != "NA" ]]; then
    awk -v x="$v" 'BEGIN{exit (x>90)?0:1}' && lvl="OK" || lvl="WARN"
  fi
  print_line "$lvl" "5) InnoDB Cache Hit Rate" "$v%" "$note"
  bump_overall "$lvl"
else
  print_line "SKIP" "5) InnoDB Cache Hit Rate" "NA" "missing status vars"
fi

# 6 Redo Log Coverage (minutes): (Uptime/60)*redo_capacity/Innodb_os_log_written
# redo capacity: prefer innodb_redo_log_capacity if exists, else (innodb_log_files_in_group*innodb_log_file_size)
if has_status "Uptime" && has_status "Innodb_os_log_written"; then
  uptime="$(num "${STATUS[Uptime]}")"
  oslog="$(num "${STATUS[Innodb_os_log_written]}")"
  redo_cap="NA"

  if has_var "innodb_redo_log_capacity"; then
    redo_cap="$(num "${VAR[innodb_redo_log_capacity]}")"
  elif has_var "innodb_log_files_in_group" && has_var "innodb_log_file_size"; then
    redo_cap="$(awk -v a="$(num "${VAR[innodb_log_files_in_group]}")" -v b="$(num "${VAR[innodb_log_file_size]}")" 'BEGIN{printf "%.0f", a*b}')"
  fi

  if [[ "$redo_cap" != "NA" ]]; then
    v="$(awk -v up="$uptime" -v cap="$redo_cap" -v w="$oslog" 'BEGIN{
      if(w==0){print "NA"; exit}
      printf "%.2f", (up/60.0)*cap/w
    }')"
    lvl="INFO"; note="target ~>= 60 minutes"
    if [[ "$v" != "NA" ]]; then
      awk -v x="$v" 'BEGIN{exit (x>=60)?0:1}' && lvl="OK" || lvl="WARN"
    fi
    print_line "$lvl" "6) Redo Log Coverage" "${v}m" "$note"
    bump_overall "$lvl"
  else
    print_line "SKIP" "6) Redo Log Coverage" "NA" "redo capacity vars missing"
  fi
else
  print_line "SKIP" "6) Redo Log Coverage" "NA" "missing status vars"
fi

# 7 DB Connection Utilization: max_used_connections*100/max_connections ; guide 0-70 OK, 70-85 WARN, 85-100 CRIT
if has_status "Max_used_connections" && has_var "max_connections"; then
  v="$(pct "${STATUS[Max_used_connections]}" "${VAR[max_connections]}")"
  lvl="INFO"; note="0-70 OK; 70-85 WARN; 85-100 CRIT"
  if [[ "$v" != "NA" ]]; then
    awk -v x="$v" 'BEGIN{exit (x<70)?0:1}' && lvl="OK" || {
      awk -v x="$v" 'BEGIN{exit (x<85)?0:1}' && lvl="WARN" || lvl="CRIT"
    }
  fi
  print_line "$lvl" "7) Connection Utilization" "$v%" "$note"
  bump_overall "$lvl"
else
  print_line "SKIP" "7) Connection Utilization" "NA" "missing status/var"
fi

# 8 Table Cache Hit Rate: prefer Table_open_cache_hits/misses if exist else Open_tables/Opened_tables
if has_status "Table_open_cache_hits" && has_status "Table_open_cache_misses"; then
  hits="$(num "${STATUS[Table_open_cache_hits]}")"
  miss="$(num "${STATUS[Table_open_cache_misses]}")"
  denom="$(awk -v h="$hits" -v m="$miss" 'BEGIN{printf "%.0f", h+m}')"
  v="$(pct "$hits" "$denom")"
  print_line "INFO" "8) Table Cache Hit Rate" "$v%" "no explicit threshold in article"
elif has_status "Open_tables" && has_status "Opened_tables"; then
  v="$(pct "${STATUS[Open_tables]}" "${STATUS[Opened_tables]}")"
  print_line "INFO" "8) Table Cache Hit Rate" "$v%" "fallback method; no explicit threshold"
else
  print_line "SKIP" "8) Table Cache Hit Rate" "NA" "missing vars"
fi

# 9 Table Definition Cache Hit Rate: Open_table_definitions/OpenED_table_definitions ; good > 75
if has_status "Open_table_definitions" && has_status "Opened_table_definitions"; then
  v="$(pct "${STATUS[Open_table_definitions]}" "${STATUS[Opened_table_definitions]}")"
  lvl="INFO"; note="good > 75"
  if [[ "$v" != "NA" ]]; then
    awk -v x="$v" 'BEGIN{exit (x>75)?0:1}' && lvl="OK" || lvl="WARN"
  fi
  print_line "$lvl" "9) Table Def Cache Hit Rate" "$v%" "$note"
  bump_overall "$lvl"
else
  print_line "SKIP" "9) Table Def Cache Hit Rate" "NA" "missing status vars"
fi

# 10 Sort Merge Passes Ratio: Sort_merge_passes*100/(Sort_scan+Sort_range)
if has_status "Sort_merge_passes" && has_status "Sort_scan" && has_status "Sort_range"; then
  denom="$(awk -v a="$(num "${STATUS[Sort_scan]}")" -v b="$(num "${STATUS[Sort_range]}")" 'BEGIN{printf "%.0f", a+b}')"
  v="$(pct "${STATUS[Sort_merge_passes]}" "$denom")"
  print_line "INFO" "10) Sort Merge Pass Ratio" "$v%" "higher can indicate sort pressure"
else
  print_line "SKIP" "10) Sort Merge Pass Ratio" "NA" "missing status vars"
fi

# 11 Temporary Disk Data: Created_tmp_disk_tables*100/Created_tmp_tables ; good <= 25
if has_status "Created_tmp_disk_tables" && has_status "Created_tmp_tables"; then
  v="$(pct "${STATUS[Created_tmp_disk_tables]}" "${STATUS[Created_tmp_tables]}")"
  lvl="INFO"; note="good <= 25"
  if [[ "$v" != "NA" ]]; then
    awk -v x="$v" 'BEGIN{exit (x<=25)?0:1}' && lvl="OK" || lvl="WARN"
  fi
  print_line "$lvl" "11) Temp Disk Tables" "$v%" "$note"
  bump_overall "$lvl"
else
  print_line "SKIP" "11) Temp Disk Tables" "NA" "missing status vars"
fi

# 12 Flushing Logs: Innodb_log_waits*100/Innodb_log_writes (lower is better)
if has_status "Innodb_log_waits" && has_status "Innodb_log_writes"; then
  v="$(pct "${STATUS[Innodb_log_waits]}" "${STATUS[Innodb_log_writes]}")"
  print_line "INFO" "12) Flushing Logs" "$v%" "lower is better"
else
  print_line "SKIP" "12) Flushing Logs" "NA" "missing status vars"
fi

# 13 Query Cache (exists on MySQL 5.6): frag=Qcache_free_blocks/Qcache_total_blocks ; delrate=Qcache_lowmem_prunes/Qcache_inserts
if has_status "Qcache_free_blocks" && has_status "Qcache_total_blocks" && has_status "Qcache_lowmem_prunes" && has_status "Qcache_inserts"; then
  frag="$(pct "${STATUS[Qcache_free_blocks]}" "${STATUS[Qcache_total_blocks]}")"
  delr="$(pct "${STATUS[Qcache_lowmem_prunes]}" "${STATUS[Qcache_inserts]}")"
  lvl="INFO"; note="frag < 10 AND deleteRate < 20"
  if [[ "$frag" != "NA" && "$delr" != "NA" ]]; then
    awk -v f="$frag" -v d="$delr" 'BEGIN{exit (f<10 && d<20)?0:1}' && lvl="OK" || lvl="WARN"
  else
    # If inserts==0, delr=NA; treat as INFO (likely QCache unused)
    lvl="INFO"
  fi
  print_line "$lvl" "13) QCache Fragmentation" "$frag%" "deleteRate=${delr}% ; $note"
  bump_overall "$lvl"
else
  print_line "SKIP" "13) QCache Fragmentation" "NA" "query cache metrics not available"
fi

# 14 CPU Utilization (sample): mysqld cores used / cpu count
cpu_counts="$(getconf _NPROCESSORS_ONLN 2>/dev/null || echo 1)"
cores_used="$(mysqld_cpu_cores_sample)"
if [[ "$cores_used" != "NA" ]]; then
  v="$(awk -v c="$cores_used" -v n="$cpu_counts" 'BEGIN{ if(n==0){print "NA"} else { printf "%.2f", (c*100.0)/n } }')"
  lvl="INFO"; note="> 100% suggests overload/contension"
  if [[ "$v" != "NA" ]]; then
    awk -v x="$v" 'BEGIN{exit (x<=100)?0:1}' && lvl="OK" || lvl="WARN"
  fi
  print_line "$lvl" "14) mysqld CPU Utilization" "$v%" "sample=${SAMPLE_SECONDS}s ; $note"
  bump_overall "$lvl"
else
  print_line "SKIP" "14) mysqld CPU Utilization" "NA" "mysqld pid not found"
fi

# 15 Memory Utilization (snapshot): used/total from free
mem_line="$(free -b 2>/dev/null | awk '/^Mem:/ {print $2"\t"$3}')"
if [[ -n "${mem_line:-}" ]]; then
  mem_total="$(echo "$mem_line" | awk '{print $1}')"
  mem_used="$(echo "$mem_line" | awk '{print $2}')"
  v="$(pct "$mem_used" "$mem_total")"
  lvl="INFO"; note="general threshold < 80"
  if [[ "$v" != "NA" ]]; then
    awk -v x="$v" 'BEGIN{exit (x<80)?0:1}' && lvl="OK" || lvl="WARN"
  fi
  print_line "$lvl" "15) Memory Utilization" "$v%" "current snapshot ; $note"
  bump_overall "$lvl"
else
  print_line "SKIP" "15) Memory Utilization" "NA" "free(1) unavailable"
fi

# 16 Disk Space Usage: datadir filesystem usage
if has_var "datadir"; then
  datadir="${VAR[datadir]}"
  df_line="$(df -P -B1 "$datadir" 2>/dev/null | awk 'NR==2 {print $2"\t"$3}')"
  if [[ -n "${df_line:-}" ]]; then
    disk_total="$(echo "$df_line" | awk '{print $1}')"
    disk_used="$(echo "$df_line" | awk '{print $2}')"
    usage_pct="$(pct "$disk_used" "$disk_total")"
    print_line "INFO" "16) Disk Space Usage" "$usage_pct%" "filesystem of datadir: $datadir"
  else
    print_line "SKIP" "16) Disk Space Usage" "NA" "df failed for datadir"
  fi
else
  print_line "SKIP" "16) Disk Space Usage" "NA" "datadir var missing"
fi

# 17 Open Files Utilization: Open_files/open_files_limit ; safe < 85
if has_status "Open_files" && has_var "open_files_limit"; then
  v="$(pct "${STATUS[Open_files]}" "${VAR[open_files_limit]}")"
  lvl="INFO"; note="safe < 85"
  if [[ "$v" != "NA" ]]; then
    awk -v x="$v" 'BEGIN{exit (x<85)?0:1}' && lvl="OK" || lvl="WARN"
  fi
  print_line "$lvl" "17) Open Files Utilization" "$v%" "$note"
  bump_overall "$lvl"
else
  print_line "SKIP" "17) Open Files Utilization" "NA" "missing status/var"
fi

# 18 Table Locking Efficiency: immediate/(immediate+waited) ; good > 95
if has_status "Table_locks_immediate" && has_status "Table_locks_waited"; then
  denom="$(awk -v a="$(num "${STATUS[Table_locks_immediate]}")" -v b="$(num "${STATUS[Table_locks_waited]}")" 'BEGIN{printf "%.0f", a+b}')"
  v="$(pct "${STATUS[Table_locks_immediate]}" "$denom")"
  lvl="INFO"; note="good > 95"
  if [[ "$v" != "NA" ]]; then
    awk -v x="$v" 'BEGIN{exit (x>95)?0:1}' && lvl="OK" || lvl="WARN"
  fi
  print_line "$lvl" "18) Table Locking Efficiency" "$v%" "$note"
  bump_overall "$lvl"
else
  print_line "SKIP" "18) Table Locking Efficiency" "NA" "missing status vars"
fi

# 19 InnoDB Dirty Pages Ratio: dirty/total ; healthy < 75
if has_status "Innodb_buffer_pool_pages_dirty" && has_status "Innodb_buffer_pool_pages_total"; then
  v="$(pct "${STATUS[Innodb_buffer_pool_pages_dirty]}" "${STATUS[Innodb_buffer_pool_pages_total]}")"
  lvl="INFO"; note="healthy < 75"
  if [[ "$v" != "NA" ]]; then
    awk -v x="$v" 'BEGIN{exit (x<75)?0:1}' && lvl="OK" || lvl="WARN"
  fi
  print_line "$lvl" "19) InnoDB Dirty Pages" "$v%" "$note"
  bump_overall "$lvl"
else
  print_line "SKIP" "19) InnoDB Dirty Pages" "NA" "missing status vars"
fi

# 20 Query Truncation: p_s history contains SQL_TEXT with '...'
if mysql_query "SELECT 1 FROM performance_schema.events_statements_history LIMIT 1" >/dev/null 2>&1; then
  qt_count="$(mysql_query "SELECT COUNT(*) FROM performance_schema.events_statements_history WHERE SQL_TEXT LIKE '%...'" 2>/dev/null | head -n1 || echo "0")"
  qt_count="$(num "$qt_count")"
  if (( qt_count > 0 )); then
    print_line "WARN" "20) Query Truncation" "TRUE" "found ${qt_count} truncated stmt(s)"
    bump_overall "WARN"
  else
    print_line "OK" "20) Query Truncation" "FALSE" "no truncated statements found"
  fi
else
  print_line "SKIP" "20) Query Truncation" "NA" "performance_schema/events history unavailable"
fi

echo
case "$overall" in
  0) echo "Overall: OK"; exit 0;;
  1) echo "Overall: WARN"; exit 1;;
  2) echo "Overall: CRIT"; exit 2;;
  *) echo "Overall: UNKNOWN"; exit 1;;
esac
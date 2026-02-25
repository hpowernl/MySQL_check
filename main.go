package main

import (
	"bufio"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/hypernode/mysql-health-check/internal/checks"
	"github.com/hypernode/mysql-health-check/internal/config"
	"github.com/hypernode/mysql-health-check/internal/db"
	"github.com/hypernode/mysql-health-check/internal/output"
)

// Version is set at build time via ldflags (e.g. -ldflags "-X main.Version=v1.0.0")
var Version = "dev"

func main() {
	cnfPath := flag.String("cnf", "/data/web/.my.cnf", "Path to .my.cnf credentials file")
	sampleSeconds := flag.Int("sample-seconds", 3, "CPU sample duration in seconds")
	noColor := flag.Bool("no-color", false, "Disable ANSI color output")
	showVersion := flag.Bool("version", false, "Show version and exit")
	flag.Parse()

	if *showVersion {
		fmt.Printf("mysql-health-check %s\n", Version)
		os.Exit(0)
	}

	if !checkDebian12() {
		fmt.Fprintln(os.Stderr, "WARNING: This tool is designed for Debian 12. Detected a different OS.")
		fmt.Fprintln(os.Stderr, "         Results may be inaccurate. Continuing anyway...")
		fmt.Fprintln(os.Stderr)
	}

	cfg, err := config.ParseMyCnf(*cnfPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "ERROR: %v\n", err)
		os.Exit(2)
	}

	m, err := db.Connect(cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "ERROR: %v\n", err)
		os.Exit(2)
	}
	defer m.Close()

	if err := m.LoadAll(); err != nil {
		fmt.Fprintf(os.Stderr, "ERROR: Failed to load MySQL data: %v\n", err)
		os.Exit(2)
	}

	categories := []checks.Category{
		{
			Name:   "System",
			Checks: checks.RunSystemChecks(m, *sampleSeconds),
		},
		{
			Name:   "MyISAM / InnoDB",
			Checks: checks.RunEngineChecks(m),
		},
		{
			Name:   "Memory",
			Checks: checks.RunCacheChecks(m),
		},
		{
			Name:   "Queries / Logs",
			Checks: checks.RunQueryChecks(m),
		},
	}

	hostname, _ := os.Hostname()

	renderer := &output.Renderer{NoColor: *noColor}
	renderer.Render(categories, m.Version, hostname, *cnfPath)

	overall := checks.OverallLevel(categories)
	switch overall {
	case checks.LevelOK:
		os.Exit(0)
	case checks.LevelWarn:
		os.Exit(1)
	case checks.LevelCrit:
		os.Exit(2)
	default:
		os.Exit(1)
	}
}

func checkDebian12() bool {
	f, err := os.Open("/etc/os-release")
	if err != nil {
		return false
	}
	defer f.Close()

	var isDebian, isVersion12 bool
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "ID=") {
			val := strings.TrimPrefix(line, "ID=")
			val = strings.Trim(val, `"`)
			isDebian = val == "debian"
		}
		if strings.HasPrefix(line, "VERSION_ID=") {
			val := strings.TrimPrefix(line, "VERSION_ID=")
			val = strings.Trim(val, `"`)
			isVersion12 = strings.HasPrefix(val, "12")
		}
	}
	return isDebian && isVersion12
}

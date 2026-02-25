package config

import (
	"bufio"
	"fmt"
	"os"
	"strings"
)

type MySQLConfig struct {
	User     string
	Password string
	Host     string
	Port     string
	Socket   string
	Database string
}

func ParseMyCnf(path string) (*MySQLConfig, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("cannot open cnf file %s: %w", path, err)
	}
	defer f.Close()

	cfg := &MySQLConfig{
		Host: "127.0.0.1",
		Port: "3306",
	}

	inClient := false
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, ";") {
			continue
		}
		if strings.HasPrefix(line, "[") {
			inClient = strings.EqualFold(line, "[client]")
			continue
		}
		if !inClient {
			continue
		}

		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}
		key := strings.TrimSpace(parts[0])
		val := strings.TrimSpace(parts[1])
		val = strings.Trim(val, `"'`)

		switch strings.ToLower(key) {
		case "user":
			cfg.User = val
		case "password":
			cfg.Password = val
		case "host":
			cfg.Host = val
		case "port":
			cfg.Port = val
		case "socket":
			cfg.Socket = val
		case "database":
			cfg.Database = val
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("error reading cnf file: %w", err)
	}

	if cfg.User == "" {
		return nil, fmt.Errorf("no user found in [client] section of %s", path)
	}
	return cfg, nil
}

func (c *MySQLConfig) DSN() string {
	db := c.Database
	if db == "" {
		db = "information_schema"
	}
	if c.Socket != "" {
		return fmt.Sprintf("%s:%s@unix(%s)/%s?timeout=10s", c.User, c.Password, c.Socket, db)
	}
	return fmt.Sprintf("%s:%s@tcp(%s:%s)/%s?timeout=10s", c.User, c.Password, c.Host, c.Port, db)
}

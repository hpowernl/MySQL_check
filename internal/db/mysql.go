package db

import (
	"database/sql"
	"fmt"
	"strings"

	_ "github.com/go-sql-driver/mysql"

	"github.com/hypernode/mysql-health-check/internal/config"
)

type MySQL struct {
	db      *sql.DB
	Status  map[string]string
	Vars    map[string]string
	Version string
}

func Connect(cfg *config.MySQLConfig) (*MySQL, error) {
	conn, err := sql.Open("mysql", cfg.DSN())
	if err != nil {
		return nil, fmt.Errorf("failed to open mysql: %w", err)
	}
	if err := conn.Ping(); err != nil {
		conn.Close()
		return nil, fmt.Errorf("failed to connect to mysql: %w", err)
	}
	return &MySQL{db: conn}, nil
}

func (m *MySQL) Close() {
	if m.db != nil {
		m.db.Close()
	}
}

func (m *MySQL) LoadAll() error {
	var err error
	m.Status, err = m.loadKeyVal("SHOW GLOBAL STATUS")
	if err != nil {
		return fmt.Errorf("SHOW GLOBAL STATUS: %w", err)
	}
	m.Vars, err = m.loadKeyVal("SHOW GLOBAL VARIABLES")
	if err != nil {
		return fmt.Errorf("SHOW GLOBAL VARIABLES: %w", err)
	}
	if err := m.db.QueryRow("SELECT VERSION()").Scan(&m.Version); err != nil {
		return fmt.Errorf("SELECT VERSION(): %w", err)
	}
	return nil
}

func (m *MySQL) loadKeyVal(query string) (map[string]string, error) {
	rows, err := m.db.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := make(map[string]string)
	var k, v string
	for rows.Next() {
		if err := rows.Scan(&k, &v); err != nil {
			return nil, err
		}
		result[k] = v
	}
	return result, rows.Err()
}

func (m *MySQL) QueryScalar(query string) (string, error) {
	var val string
	err := m.db.QueryRow(query).Scan(&val)
	if err != nil {
		return "", err
	}
	return val, nil
}

func (m *MySQL) VersionAtLeast(major, minor, patch int) bool {
	v := m.Version
	if idx := strings.Index(v, "-"); idx >= 0 {
		v = v[:idx]
	}
	var ma, mi, pa int
	fmt.Sscanf(v, "%d.%d.%d", &ma, &mi, &pa)
	if ma != major {
		return ma > major
	}
	if mi != minor {
		return mi > minor
	}
	return pa >= patch
}

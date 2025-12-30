package trillianUtils

import (
	"fmt"
	"net"

	"github.com/go-sql-driver/mysql"
)

type MySQLOptions struct {
	Host       string
	Port       string
	User       string
	Password   string
	Database   string
	TlsEnabled bool
}

func ParseMySQL(dsn string) (*MySQLOptions, error) {
	cfg, err := mysql.ParseDSN(dsn)
	if err != nil {
		return nil, fmt.Errorf("can't parse mysql dsn: %w", err)
	}

	opts := &MySQLOptions{
		User:     cfg.User,
		Password: cfg.Passwd,
		Host:     cfg.Addr,
		Database: cfg.DBName,
	}

	// Split host:port if present
	if host, port, err := net.SplitHostPort(cfg.Addr); err == nil {
		opts.Host = host
		opts.Port = port
	}

	// TLS enabled if configured
	if cfg.TLSConfig != "" {
		opts.TlsEnabled = true
	}

	return opts, nil
}

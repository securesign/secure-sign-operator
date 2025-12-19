package trillianUtils

import (
	"fmt"
	"net/url"
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
	u, err := url.Parse(dsn)
	if err != nil {
		return nil, fmt.Errorf("can't parse mysql dsn: %w", err)
	}

	if u.Hostname() == "" {
		return nil, fmt.Errorf("mysql host is empty")
	}

	opts := &MySQLOptions{
		Host: u.Hostname(),
	}

	if u.Port() != "" {
		opts.Port = u.Port()
	}

	// Extract username/password
	if u.User != nil {
		opts.User = u.User.Username()

		if p, ok := u.User.Password(); ok {
			opts.Password = p
		}
	}

	// Remove leading "/"
	if db := u.Path; len(db) > 1 {
		opts.Database = db[1:]
	}

	// TLS based on scheme or query parameters
	if u.Scheme == "mysqls" {
		opts.TlsEnabled = true
	} else if q := u.Query().Get("tls"); q == "true" || q == "1" {
		opts.TlsEnabled = true
	}

	return opts, nil
}

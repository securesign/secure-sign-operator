package redis

import (
	"fmt"
	"net/url"
)

type RedisOptions struct {
	Host, Port, Password string
	TlsEnabled           bool
}

func Parse(dsn string) (options *RedisOptions, err error) {
	// go-redis uses url.Parse internally so there is no problem to use it to parse redis DSN (see https://github.com/redis/go-redis/blob/v9.10.0/options.go#L349)
	searchIndexUrl, err := url.Parse(dsn)
	if err != nil {
		return nil, fmt.Errorf("can't parse redis searchIndex url: %w", err)
	}
	if searchIndexUrl.Hostname() == "" {
		return nil, fmt.Errorf("searchIndex url Host is empty")
	}

	options = &RedisOptions{Host: searchIndexUrl.Hostname()}

	if searchIndexUrl.Port() != "" {
		options.Port = searchIndexUrl.Port()
	}

	if searchIndexUrl.Scheme == "rediss" {
		options.TlsEnabled = true
	}

	if searchIndexUrl.User != nil {
		if p, ok := searchIndexUrl.User.Password(); ok {
			options.Password = p
		}
	}

	return
}

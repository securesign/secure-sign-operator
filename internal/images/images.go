package images

import (
	"bufio"
	"bytes"
	"embed"
	"fmt"
	"strings"
	"sync"
)

type Image string

const (
	TrillianLogSigner  Image = "RELATED_IMAGE_TRILLIAN_LOG_SIGNER"
	TrillianServer     Image = "RELATED_IMAGE_TRILLIAN_LOG_SERVER"
	TrillianDb         Image = "RELATED_IMAGE_TRILLIAN_DB"
	TrillianNetcat     Image = "RELATED_IMAGE_TRILLIAN_NETCAT"
	TrillianCreateTree Image = "RELATED_IMAGE_CREATETREE"

	FulcioServer Image = "RELATED_IMAGE_FULCIO_SERVER"

	RekorRedis    Image = "RELATED_IMAGE_REKOR_REDIS"
	RekorServer   Image = "RELATED_IMAGE_REKOR_SERVER"
	RekorSearchUi Image = "RELATED_IMAGE_REKOR_SEARCH_UI"
	RekorMonitor  Image = "RELATED_IMAGE_REKOR_MONITOR"
	BackfillRedis Image = "RELATED_IMAGE_BACKFILL_REDIS"

	Tuf Image = "RELATED_IMAGE_TUF"

	CTLog Image = "RELATED_IMAGE_CTLOG"

	TimestampAuthority Image = "RELATED_IMAGE_TIMESTAMP_AUTHORITY"

	HttpServer   Image = "RELATED_IMAGE_HTTP_SERVER"
	ClientServer Image = "RELATED_IMAGE_CLIENT_SERVER"
)

var Images = []Image{
	TrillianLogSigner,
	TrillianServer,
	TrillianDb,
	TrillianNetcat,
	TrillianCreateTree,
	FulcioServer,
	RekorRedis,
	RekorServer,
	RekorSearchUi,
	RekorMonitor,
	BackfillRedis,
	Tuf,
	CTLog,
	TimestampAuthority,
	HttpServer,
	ClientServer,
}

//go:generate cp ../../config/default/images.env embed/images.env
//go:embed embed/*
var content embed.FS

var Registry registry

func init() {
	fd, err := content.ReadFile("embed/images.env")
	if err != nil {
		panic(err)
	}

	data, err := parseConfigFile(fd)
	if err != nil {
		panic(err)
	}

	Registry = registry{
		data: data,
	}

}

// parseConfigFile parses an embedded `.env` content and returns a map of key-value pairs.
func parseConfigFile(envContent []byte) (map[Image]string, error) {
	data := make(map[Image]string)
	scanner := bufio.NewScanner(bytes.NewReader(envContent))

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		// Skip empty lines or comments
		if len(line) == 0 || strings.HasPrefix(line, "#") {
			continue
		}

		// Split the line into key and value
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			return nil, fmt.Errorf("invalid line format: %s", line)
		}

		key := strings.TrimSpace(parts[0])
		value := strings.TrimSpace(parts[1])
		data[Image(key)] = value
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("error reading env content: %w", err)
	}

	return data, nil
}

type registry struct {
	mutex sync.RWMutex
	data  map[Image]string
}

func (r *registry) Get(name Image) string {
	r.mutex.RLock()
	defer r.mutex.RUnlock()
	return r.data[name]
}

func (r *registry) Set(name Image, value string) {
	r.mutex.Lock()
	defer r.mutex.Unlock()
	r.data[name] = value
}

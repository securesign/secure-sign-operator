package support

import (
	"archive/tar"
	"compress/gzip"
	"fmt"
	"io"
	"os"
)

type logTarget struct {
	reader io.Reader
	size   int64
}

func createArchive(file *os.File, logs map[string]logTarget) error {
	defer func() { _ = file.Close() }()

	// Create a new gzip writer
	gzipWriter := gzip.NewWriter(file)
	defer func() { _ = gzipWriter.Close() }()

	// Create a new tar writer
	tarWriter := tar.NewWriter(gzipWriter)
	defer func() { _ = tarWriter.Close() }()

	// Iterate over the logs map
	for componentName, log := range logs {

		// Create a tar header for the file
		tarHeader := &tar.Header{
			Name: componentName,
			Mode: 0600,
			Size: log.size,
		}

		// Write the header to the tar file
		if err := tarWriter.WriteHeader(tarHeader); err != nil {
			return fmt.Errorf("tar write header: %w", err)
		}

		// Copy the logTarget data to the tar file
		if _, err := io.Copy(tarWriter, log.reader); err != nil {
			return fmt.Errorf("tar write content: %w", err)
		}
	}
	return nil
}

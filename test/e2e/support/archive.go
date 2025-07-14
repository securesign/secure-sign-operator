package support

import (
	"archive/tar"
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
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

func Untar(dst string, r io.Reader) error {

	tr := tar.NewReader(r)

	for {
		header, err := tr.Next()

		switch {

		// if no more files are found return
		case err == io.EOF:
			return nil

		// return any other error
		case err != nil:
			return err

		// if the header is nil, just skip it (not sure how this happens)
		case header == nil:
			continue
		}

		// the target location where the dir/file should be created
		target := filepath.Join(dst, header.Name)

		// the following switch could also be done using fi.Mode(), not sure if there
		// a benefit of using one vs. the other.
		// fi := header.FileInfo()

		// check the file type
		switch header.Typeflag {

		// if its a dir and it doesn't exist create it
		case tar.TypeDir:
			if _, err := os.Stat(target); err != nil {
				if err := os.MkdirAll(target, 0755); err != nil {
					return err
				}
			}

		// if it's a file create it
		case tar.TypeReg:
			f, err := os.OpenFile(target, os.O_CREATE|os.O_RDWR, os.FileMode(header.Mode))
			if err != nil {
				return err
			}

			// copy over contents
			if _, err := io.Copy(f, tr); err != nil {
				return err
			}

			// manually close here after each file operation; defering would cause each file close
			// to wait until all operations have completed.
			_ = f.Close()
		}
	}
}

func Tar(src string, writer io.Writer) error {

	// ensure the src actually exists before trying to tar it
	if _, err := os.Stat(src); err != nil {
		return fmt.Errorf("unable to tar files - %v", err.Error())
	}

	tw := tar.NewWriter(writer)
	defer func() { _ = tw.Close() }()

	// walk path
	return filepath.Walk(src, func(file string, fi os.FileInfo, err error) error {

		// return on any error
		if err != nil {
			return err
		}

		// return on non-regular files (thanks to [kumo](https://medium.com/@komuw/just-like-you-did-fbdd7df829d3) for this suggested update)
		if !fi.Mode().IsRegular() {
			return nil
		}

		// create a new dir/file header
		header, err := tar.FileInfoHeader(fi, fi.Name())
		if err != nil {
			return err
		}

		// update the name to correctly reflect the desired destination when untaring
		header.Name = strings.TrimPrefix(strings.ReplaceAll(file, src, ""), string(filepath.Separator))

		// write the header
		if err := tw.WriteHeader(header); err != nil {
			return err
		}

		// open files for taring
		f, err := os.Open(file)
		if err != nil {
			return err
		}

		// copy file data into tar writer
		if _, err := io.Copy(tw, f); err != nil {
			return err
		}

		// manually close here after each file operation; defering would cause each file close
		// to wait until all operations have completed.
		_ = f.Close()

		return nil
	})
}

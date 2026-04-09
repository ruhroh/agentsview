package importer

import (
	"archive/zip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// ExtractZip extracts a zip file to a temporary directory.
// Returns the directory path and a cleanup function that
// removes it. The caller must call cleanup when done.
func ExtractZip(zipPath string) (string, func(), error) {
	r, err := zip.OpenReader(zipPath)
	if err != nil {
		return "", nil, fmt.Errorf("opening zip %s: %w", zipPath, err)
	}
	defer r.Close()

	dir, err := os.MkdirTemp("", "agentsview-import-*")
	if err != nil {
		return "", nil, fmt.Errorf("creating temp dir: %w", err)
	}
	cleanup := func() { os.RemoveAll(dir) }

	for _, f := range r.File {
		if err := extractZipFile(dir, f); err != nil {
			cleanup()
			return "", nil, fmt.Errorf("extracting %s: %w", f.Name, err)
		}
	}

	return dir, cleanup, nil
}

func extractZipFile(destDir string, f *zip.File) error {
	dest := filepath.Join(destDir, f.Name)
	if !strings.HasPrefix(
		filepath.Clean(dest),
		filepath.Clean(destDir)+string(os.PathSeparator),
	) {
		return fmt.Errorf("illegal path in zip: %s", f.Name)
	}

	if f.FileInfo().IsDir() {
		return os.MkdirAll(dest, 0o755)
	}

	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return err
	}

	rc, err := f.Open()
	if err != nil {
		return err
	}
	defer rc.Close()

	out, err := os.Create(dest)
	if err != nil {
		return err
	}

	if _, err = io.Copy(out, rc); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}

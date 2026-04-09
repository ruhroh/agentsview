package importer

import (
	"crypto/sha256"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// AssetIndex maps asset pointer prefixes to file paths on disk.
type AssetIndex struct {
	entries map[string]string
}

// BuildAssetIndex scans an export directory for image assets.
// ChatGPT exports store images in three locations:
//   - dalle-generations/ (DALL-E outputs)
//   - user-*/ directories (user uploads via sediment://)
//   - root directory as file-* (user uploads via file-service://)
func BuildAssetIndex(exportDir string) AssetIndex {
	idx := AssetIndex{entries: make(map[string]string)}

	// DALL-E generated images.
	dalleDir := filepath.Join(exportDir, "dalle-generations")
	scanAssetDir(dalleDir, idx.entries)

	// User uploads in user-*/ subdirectories.
	matches, _ := filepath.Glob(filepath.Join(exportDir, "user-*"))
	for _, m := range matches {
		info, err := os.Stat(m)
		if err == nil && info.IsDir() {
			scanAssetDir(m, idx.entries)
		}
	}

	// User uploads as loose file-* in the root directory.
	scanAssetFiles(exportDir, idx.entries)

	return idx
}

func scanAssetDir(dir string, entries map[string]string) {
	files, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	for _, f := range files {
		if f.IsDir() {
			continue
		}
		name := f.Name()
		prefix := extractAssetPrefix(name)
		if prefix != "" {
			entries[prefix] = filepath.Join(dir, name)
		}
	}
}

// scanAssetFiles indexes only file-* and file_* entries in a
// directory. Used for the root dir where non-asset files exist.
func scanAssetFiles(dir string, entries map[string]string) {
	files, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	for _, f := range files {
		if f.IsDir() {
			continue
		}
		name := f.Name()
		if !strings.HasPrefix(name, "file-") &&
			!strings.HasPrefix(name, "file_") {
			continue
		}
		prefix := extractAssetPrefix(name)
		if prefix != "" {
			entries[prefix] = filepath.Join(dir, name)
		}
	}
}

// extractAssetPrefix gets the stable prefix from filenames like
// "file-abc123-aaaa-bbbb-cccc-dddddddddddd.webp"
func extractAssetPrefix(name string) string {
	base := strings.TrimSuffix(name, filepath.Ext(name))
	// UUID is 36 chars at end, preceded by hyphen = 37 chars total
	if len(base) > 37 && base[len(base)-37] == '-' {
		return base[:len(base)-37]
	}
	return base
}

// Resolve maps an asset pointer URL to a file path.
func (idx AssetIndex) Resolve(pointer string) (string, bool) {
	var prefix string
	switch {
	case strings.HasPrefix(pointer, "file-service://"):
		prefix = strings.TrimPrefix(pointer, "file-service://")
	case strings.HasPrefix(pointer, "sediment://"):
		prefix = strings.TrimPrefix(pointer, "sediment://")
	default:
		return "", false
	}
	path, ok := idx.entries[prefix]
	return path, ok
}

// allowedImageExts is the set of passive image formats that
// are safe to serve inline. Active content (svg, html, js) is
// rejected to prevent stored XSS.
var allowedImageExts = map[string]bool{
	".png":  true,
	".jpg":  true,
	".jpeg": true,
	".webp": true,
	".gif":  true,
}

// CopyAsset copies a file to the assets directory using its
// SHA-256 hash as the filename. Returns the asset:// reference.
// Only passive image types are accepted; active content is
// rejected.
func CopyAsset(srcPath, assetsDir string) (string, error) {
	ext := strings.ToLower(filepath.Ext(srcPath))
	if !allowedImageExts[ext] {
		return "", fmt.Errorf(
			"unsupported asset type: %s", ext,
		)
	}

	f, err := os.Open(srcPath)
	if err != nil {
		return "", fmt.Errorf("reading asset: %w", err)
	}
	defer f.Close()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", fmt.Errorf("hashing asset: %w", err)
	}

	hash := fmt.Sprintf("%x", h.Sum(nil))
	filename := hash + ext
	destPath := filepath.Join(assetsDir, filename)

	if _, err := os.Stat(destPath); err == nil {
		return "asset://" + filename, nil
	}

	if err := os.MkdirAll(assetsDir, 0o755); err != nil {
		return "", fmt.Errorf("creating assets dir: %w", err)
	}

	// Re-read source for copy (we consumed it for hashing).
	src, err := os.Open(srcPath)
	if err != nil {
		return "", fmt.Errorf("reopening asset: %w", err)
	}
	defer src.Close()

	out, err := os.Create(destPath)
	if err != nil {
		return "", fmt.Errorf("writing asset: %w", err)
	}

	if _, err := io.Copy(out, src); err != nil {
		out.Close()
		return "", fmt.Errorf("copying asset: %w", err)
	}
	if err := out.Close(); err != nil {
		return "", fmt.Errorf("closing asset: %w", err)
	}

	return "asset://" + filename, nil
}

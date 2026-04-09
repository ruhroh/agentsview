package importer

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuildAssetIndex(t *testing.T) {
	dir := t.TempDir()

	dalleDir := filepath.Join(dir, "dalle-generations")
	require.NoError(t, os.MkdirAll(dalleDir, 0o755))
	require.NoError(t, os.WriteFile(
		filepath.Join(dalleDir, "file-abc123-aaaa1111-bbbb-cccc-dddd-eeeeeeeeeeee.webp"),
		[]byte("fake image"), 0o644,
	))

	userDir := filepath.Join(dir, "user-xyz")
	require.NoError(t, os.MkdirAll(userDir, 0o755))
	require.NoError(t, os.WriteFile(
		filepath.Join(userDir, "file_deadbeef-aaaa1111-bbbb-cccc-dddd-eeeeeeeeeeee.png"),
		[]byte("fake upload"), 0o644,
	))

	idx := BuildAssetIndex(dir)

	path, ok := idx.Resolve("file-service://file-abc123")
	assert.True(t, ok)
	assert.Contains(t, path, "file-abc123")

	path, ok = idx.Resolve("sediment://file_deadbeef")
	assert.True(t, ok)
	assert.Contains(t, path, "file_deadbeef")

	_, ok = idx.Resolve("file-service://file-unknown")
	assert.False(t, ok)
}

func TestCopyAsset(t *testing.T) {
	src := filepath.Join(t.TempDir(), "source.webp")
	require.NoError(t, os.WriteFile(src, []byte("image data"), 0o644))

	assetsDir := filepath.Join(t.TempDir(), "assets")

	ref, err := CopyAsset(src, assetsDir)
	require.NoError(t, err)
	assert.Contains(t, ref, "asset://")
	assert.Contains(t, ref, ".webp")

	// Second call should return same ref (dedup).
	ref2, err := CopyAsset(src, assetsDir)
	require.NoError(t, err)
	assert.Equal(t, ref, ref2)

	entries, err := os.ReadDir(assetsDir)
	require.NoError(t, err)
	assert.Len(t, entries, 1)
}

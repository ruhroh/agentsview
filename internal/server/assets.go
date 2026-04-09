package server

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

// safeImageTypes maps extensions to MIME types for passive
// image formats. Active content (svg, html, js) is never
// served inline to prevent stored XSS.
var safeImageTypes = map[string]string{
	".png":  "image/png",
	".jpg":  "image/jpeg",
	".jpeg": "image/jpeg",
	".webp": "image/webp",
	".gif":  "image/gif",
}

func (s *Server) handleGetAsset(
	w http.ResponseWriter, r *http.Request,
) {
	filename := r.PathValue("filename")
	if filename == "" {
		writeError(w, http.StatusBadRequest, "missing filename")
		return
	}

	if strings.Contains(filename, "..") ||
		strings.Contains(filename, "/") ||
		strings.Contains(filename, "\\") {
		writeError(w, http.StatusBadRequest, "invalid filename")
		return
	}

	ext := strings.ToLower(filepath.Ext(filename))
	contentType, ok := safeImageTypes[ext]
	if !ok {
		writeError(w, http.StatusForbidden,
			"unsupported asset type")
		return
	}

	assetsDir := filepath.Join(s.cfg.DataDir, "assets")
	filePath := filepath.Join(assetsDir, filename)

	if _, err := os.Stat(filePath); err != nil {
		writeError(w, http.StatusNotFound, "asset not found")
		return
	}

	w.Header().Set("Content-Type", contentType)
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Cache-Control",
		"public, max-age=31536000, immutable")

	http.ServeFile(w, r, filePath)
}

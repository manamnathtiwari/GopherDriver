// Package hasher provides streaming SHA256 file hashing and metadata extraction.
package hasher

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

// Metadata holds computed file metadata.
type Metadata struct {
	Hash      string                 // hex-encoded SHA256
	Size      int64                  // file size in bytes
	Extension string                 // file extension
	Extra     map[string]interface{} // Rich metadata (mime, width, height, etc.)
}

// ComputeMetadata streams the file through SHA256 and returns its metadata.
func ComputeMetadata(filePath string) (*Metadata, error) {
	f, err := os.Open(filePath)
	if err != nil {
		return nil, fmt.Errorf("hasher: open file: %w", err)
	}
	defer f.Close()

	// 1. Setup SHA256 hasher
	h := sha256.New()

	// 2. Read first 512 bytes for MIME detection
	head := make([]byte, 512)
	n, err := f.Read(head)
	if err != nil && err != io.EOF {
		return nil, fmt.Errorf("hasher: read head: %w", err)
	}

	mimeType := http.DetectContentType(head[:n])

	// Reset file pointer depending on how much we read
	// Actually, we can just MultiReader the head + rest of file
	// But seeking is easier since it's a file
	if _, err := f.Seek(0, 0); err != nil {
		return nil, fmt.Errorf("hasher: seek: %w", err)
	}

	// 3. Compute Hash & Size (Stream)
	size, err := io.Copy(h, f)
	if err != nil {
		return nil, fmt.Errorf("hasher: copy: %w", err)
	}
	hash := hex.EncodeToString(h.Sum(nil))

	extra := map[string]interface{}{
		"mime_type": mimeType,
	}

	// 4. Content-Specific Analysis
	// Re-open file for specific analysis to avoid seek issues or complex readers
	if strings.HasPrefix(mimeType, "image/") {
		if imgArgs, err := analyzeImage(filePath); err == nil {
			for k, v := range imgArgs {
				extra[k] = v
			}
		}
	} else if strings.HasPrefix(mimeType, "text/") {
		if txtArgs, err := analyzeText(filePath); err == nil {
			for k, v := range txtArgs {
				extra[k] = v
			}
		}
	}

	return &Metadata{
		Hash:      hash,
		Size:      size,
		Extension: filepath.Ext(filePath),
		Extra:     extra,
	}, nil
}

func analyzeImage(path string) (map[string]interface{}, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	cfg, _, err := image.DecodeConfig(f)
	if err != nil {
		return nil, err
	}
	return map[string]interface{}{
		"width":  cfg.Width,
		"height": cfg.Height,
	}, nil
}

func analyzeText(path string) (map[string]interface{}, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	lines := 0
	words := 0
	for scanner.Scan() {
		lines++
		words += len(bytes.Fields(scanner.Bytes()))
	}
	return map[string]interface{}{
		"lines": lines,
		"words": words,
	}, nil
}

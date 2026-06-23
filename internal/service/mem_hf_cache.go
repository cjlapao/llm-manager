package service

import (
	"os"
	"path/filepath"
	"strings"

	config "github.com/user/llm-manager/internal/config"
)

// HFCacheInfo holds information about a model's HF cache.
type HFCacheInfo struct {
	Cached  bool
	Size    int64
	Files   int
	SnapDir string
}

var _ = config.Config{} // ensure config dependency

// HFCacheSize returns detailed information about a model's HF cache
// including whether it's cached, total size, and file count.
func (s *ContainerService) HFCacheSize(hfRepo string) HFCacheInfo {
	info := HFCacheInfo{}
	if hfRepo == "" {
		return info
	}

	cacheDir := "models--" + strings.ReplaceAll(hfRepo, "/", "--")

	// Try hub/ subdirectory first (standard HF cache layout)
	baseDir := filepath.Join(s.cfg.HFCacheDir, "hub", cacheDir)
	if !dirExists(baseDir) {
		// Fall back to direct path under HFCacheDir (legacy/non-standard)
		baseDir = filepath.Join(s.cfg.HFCacheDir, cacheDir)
	}

	if !dirExists(baseDir) {
		return info
	}

	// Collect all snapshot dirs
	var snapshotDirs []string
	hubSnapshots := filepath.Join(baseDir, "snapshots")
	if dirExists(hubSnapshots) {
		entries, err := os.ReadDir(hubSnapshots)
		if err == nil {
			for _, entry := range entries {
				if entry.IsDir() {
					snapshotDirs = append(snapshotDirs, filepath.Join(hubSnapshots, entry.Name()))
				}
			}
		}
	}

	if len(snapshotDirs) == 0 {
		return info
	}

	// Walk the snapshot directories to compute size and count files
	var totalSize int64
	var fileCount int
	var foundConfig bool

	for _, snapDir := range snapshotDirs {
		walkErr := filepath.Walk(snapDir, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return nil
			}
			if !info.IsDir() {
				totalSize += info.Size()
				fileCount++
				if info.Name() == "config.json" {
					foundConfig = true
				}
			}
			return nil
		})
		if walkErr != nil {
			continue
		}
	}

	// Also count blobs (actual weight files in .cache/hub/blobs)
	blobDir := filepath.Join(baseDir, "blobs")
	if dirExists(blobDir) {
		blobEntries, err := os.ReadDir(blobDir)
		if err == nil {
			for _, entry := range blobEntries {
				if !entry.IsDir() {
					fi, err := os.Stat(filepath.Join(blobDir, entry.Name()))
					if err == nil {
						totalSize += fi.Size()
						fileCount++
					}
				}
			}
		}
	}

	info.Cached = foundConfig
	info.Size = totalSize
	info.Files = fileCount
	if len(snapshotDirs) > 0 {
		info.SnapDir = snapshotDirs[0]
	}

	return info
}

// IsHFCached is deprecated; use HFCacheSize instead.
func (s *ContainerService) IsHFCached(hfRepo string) bool {
	return s.HFCacheSize(hfRepo).Cached
}

// dirExists checks if a directory exists.
func dirExists(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	return info.IsDir()
}

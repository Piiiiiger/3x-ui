// Package frontendassets keeps hashed bundles available to tabs opened before an upgrade.
package frontendassets

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/mhsanaei/3x-ui/v3/internal/config"
	"github.com/mhsanaei/3x-ui/v3/internal/logger"
)

var mu sync.Mutex

const retention = 7 * 24 * time.Hour
const budget int64 = 256 << 20

// Mount serves the current build first, with a bounded on-disk archive as fallback.
func Mount(current fs.FS) fs.FS {
	dir := filepath.Join(config.GetDBFolderPath(), "frontend-assets")
	mu.Lock()
	err := archive(current, dir, time.Now(), budget)
	mu.Unlock()
	if err != nil {
		logger.Warning("frontend asset archive:", err)
	}
	return &fallback{current: current, old: os.DirFS(dir)}
}

type fallback struct{ current, old fs.FS }

func (f *fallback) Open(name string) (fs.File, error) {
	if !fs.ValidPath(name) || name == "." || strings.Contains(name, "/") {
		return nil, fs.ErrNotExist
	}
	file, err := f.current.Open(name)
	if !errors.Is(err, fs.ErrNotExist) {
		return file, err
	}
	return f.old.Open(name)
}
func archive(current fs.FS, dir string, now time.Time, maxBytes int64) error {
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	entries, err := fs.ReadDir(current, ".")
	if err != nil {
		return err
	}
	live := map[string]bool{}
	for _, entry := range entries {
		if !entry.Type().IsRegular() {
			continue
		}
		name := entry.Name()
		live[name] = true
		target := filepath.Join(dir, name)
		if _, err := os.Stat(target); errors.Is(err, fs.ErrNotExist) {
			data, err := fs.ReadFile(current, name)
			if err != nil {
				return err
			}
			tmp, err := os.CreateTemp(dir, ".asset-")
			if err != nil {
				return err
			}
			tmpName := tmp.Name()
			_, writeErr := tmp.Write(data)
			closeErr := tmp.Close()
			if writeErr != nil || closeErr != nil {
				os.Remove(tmpName)
				return errors.Join(writeErr, closeErr)
			}
			if err := os.Rename(tmpName, target); err != nil {
				os.Remove(tmpName)
				return err
			}
		} else if err != nil {
			return err
		}
		if err := os.Chtimes(target, now, now); err != nil {
			return err
		}
	}
	entries, err = os.ReadDir(dir)
	if err != nil {
		return err
	}
	var old []fs.FileInfo
	var size int64
	for _, entry := range entries {
		if !entry.Type().IsRegular() {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if !live[info.Name()] && now.Sub(info.ModTime()) > retention {
			if err := os.Remove(filepath.Join(dir, info.Name())); err != nil {
				return err
			}
			continue
		}
		size += info.Size()
		if !live[info.Name()] {
			old = append(old, info)
		}
	}
	sort.Slice(old, func(i, j int) bool { return old[i].ModTime().Before(old[j].ModTime()) })
	for _, info := range old {
		if size <= maxBytes {
			break
		}
		if err := os.Remove(filepath.Join(dir, info.Name())); err != nil {
			return err
		}
		size -= info.Size()
	}
	return nil
}

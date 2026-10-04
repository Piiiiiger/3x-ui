package frontendassets

import (
	"io/fs"
	"os"
	"path/filepath"
	"testing"
	"testing/fstest"
	"time"
)

func TestArchiveUpgradeAndBounds(t *testing.T) {
	dir := t.TempDir()
	now := time.Now()
	before := fstest.MapFS{"old-hash.js": {Data: []byte("old")}, "shared.js": {Data: []byte("one")}}
	after := fstest.MapFS{"new-hash.js": {Data: []byte("new")}, "shared.js": {Data: []byte("two")}}
	if err := archive(before, dir, now, 100); err != nil {
		t.Fatal(err)
	}
	if err := archive(after, dir, now.Add(time.Hour), 100); err != nil {
		t.Fatal(err)
	}
	merged := &fallback{after, os.DirFS(dir)}
	for name, want := range map[string]string{"old-hash.js": "old", "new-hash.js": "new", "shared.js": "two"} {
		data, err := fs.ReadFile(merged, name)
		if err != nil || string(data) != want {
			t.Fatalf("%s: %s %v", name, data, err)
		}
	}
	for _, name := range []string{"../secret", ".", "/old-hash.js"} {
		if _, err := merged.Open(name); err == nil {
			t.Fatal("invalid path accepted")
		}
	}
	if err := archive(after, dir, now.Add(retention+time.Hour), 100); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "old-hash.js")); !os.IsNotExist(err) {
		t.Fatal("expired asset retained")
	}
	if err := archive(before, dir, now, 100); err != nil {
		t.Fatal(err)
	}
	if err := archive(after, dir, now.Add(time.Hour), 6); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "old-hash.js")); !os.IsNotExist(err) {
		t.Fatal("budget not enforced")
	}
}

package dubi

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"testing"
	"testing/fstest"
)

func TestAssetFilesystem(t *testing.T) {
	previous := AssetFS()
	t.Cleanup(func() { AssetFS(previous) })
	AssetFS(nil)
	if AssetFS() != nil {
		t.Fatal("resource filesystem was not cleared")
	}
	if _, err := AssetFile("public/index.html"); !errors.Is(err, errAssetFSMissing) {
		t.Fatalf("missing filesystem file error = %v", err)
	}
	if _, err := AssetDir("public"); !errors.Is(err, errAssetFSMissing) {
		t.Fatalf("missing filesystem directory error = %v", err)
	}
	if _, err := AssetStat("public/index.html"); !errors.Is(err, errAssetFSMissing) {
		t.Fatalf("missing filesystem stat error = %v", err)
	}

	files := fstest.MapFS{"public/index.html": {Data: []byte("hello")}}
	if AssetFS(files) == nil {
		t.Fatal("setter did not return the filesystem")
	}
	data, err := AssetFile("public/index.html")
	if err != nil || string(data) != "hello" {
		t.Fatalf("file = %q, %v", data, err)
	}
	entries, err := AssetDir("public")
	if err != nil || len(entries) != 1 || entries[0].Name() != "index.html" {
		t.Fatalf("directory = %v, %v", entries, err)
	}
	info, err := AssetStat("public/index.html")
	if err != nil || info.Size() != 5 || !info.Mode().IsRegular() {
		t.Fatalf("stat = %v, %v", info, err)
	}
	for _, name := range []string{"missing", "../public/index.html", "/public/index.html"} {
		if _, err := AssetFile(name); err == nil {
			t.Errorf("AssetFile(%q) should fail", name)
		}
	}

	// A resource filesystem can be replaced without Registry or lifecycle work.
	directory := t.TempDir()
	if err := os.WriteFile(filepath.Join(directory, "hello.txt"), []byte("disk"), 0600); err != nil {
		t.Fatal(err)
	}
	AssetFS(os.DirFS(directory))
	data, err = AssetFile("hello.txt")
	if err != nil || string(data) != "disk" {
		t.Fatalf("replacement file = %q, %v", data, err)
	}
	if _, err := AssetFile("public/index.html"); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("replaced file error = %v", err)
	}
}

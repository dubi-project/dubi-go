package dubi

import (
	"errors"
	"io/fs"
	"sync"
)

var (
	errAssetFSMissing = errors.New("asset fs not set")
	asset             struct {
		mutex sync.RWMutex
		fsys  fs.FS
	}
)

// AssetFS gets the process-wide resource filesystem. Pass a filesystem to set
// it, or nil to clear it. Resources are independent of the Registry and Profile.
// Applications may supply embed.FS, os.DirFS, or any other fs.FS implementation.
func AssetFS(fss ...fs.FS) fs.FS {
	if len(fss) > 0 {
		asset.mutex.Lock()
		asset.fsys = fss[0]
		asset.mutex.Unlock()
		return fss[0]
	}
	asset.mutex.RLock()
	defer asset.mutex.RUnlock()
	return asset.fsys
}

// AssetDir reads a directory from the resource filesystem.
func AssetDir(name string) ([]fs.DirEntry, error) {
	fsys := AssetFS()
	if fsys == nil {
		return nil, errAssetFSMissing
	}
	return fs.ReadDir(fsys, name)
}

// AssetFile reads a file from the resource filesystem.
func AssetFile(name string) ([]byte, error) {
	fsys := AssetFS()
	if fsys == nil {
		return nil, errAssetFSMissing
	}
	return fs.ReadFile(fsys, name)
}

// AssetStat returns file information from the resource filesystem.
func AssetStat(name string) (fs.FileInfo, error) {
	fsys := AssetFS()
	if fsys == nil {
		return nil, errAssetFSMissing
	}
	return fs.Stat(fsys, name)
}

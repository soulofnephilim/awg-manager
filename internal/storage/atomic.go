package storage

import (
	"fmt"
	"os"
	"path/filepath"
	"time"
)

const FilePermission = 0644
const DirPermission = 0755

// AtomicWrite writes data to path atomically using temp file + rename.
func AtomicWrite(path string, data []byte) error {
	return AtomicWritePerm(path, data, FilePermission)
}

// AtomicWritePerm is like AtomicWrite but with custom file permissions.
//
// The temp file and the containing directory are fsync'ed before/after the
// rename: on routers state lives on flash with ext4 delayed allocation, and
// without the syncs a power loss inside the writeback window can leave a
// zero-length file under the final name.
func AtomicWritePerm(path string, data []byte, perm os.FileMode) error {
	dir := filepath.Dir(path)

	if err := os.MkdirAll(dir, DirPermission); err != nil {
		return fmt.Errorf("create directory %s: %w", dir, err)
	}

	tmpPath := fmt.Sprintf("%s.tmp.%d.%d", path, os.Getpid(), time.Now().UnixNano())

	if err := writeFileSync(tmpPath, data, perm); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("write temp file: %w", err)
	}

	if err := os.Rename(tmpPath, path); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("rename temp to target: %w", err)
	}

	syncDir(dir)
	return nil
}

func writeFileSync(path string, data []byte, perm os.FileMode) error {
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, perm)
	if err != nil {
		return err
	}
	if _, err := f.Write(data); err != nil {
		f.Close()
		return err
	}
	if err := f.Sync(); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}

// syncDir fsyncs a directory so a preceding rename survives power loss.
// Best-effort: some filesystems reject directory fsync.
func syncDir(dir string) {
	d, err := os.Open(dir)
	if err != nil {
		return
	}
	d.Sync()
	d.Close()
}

// QuarantineCorrupt sets a corrupt state file aside as <path>.corrupt and
// logs the event. Stores that would otherwise silently reset to defaults —
// and then persist the wipe over the recoverable file on the next save —
// call this so the user's data survives for manual recovery.
func QuarantineCorrupt(path string, parseErr error) {
	quarantine := path + ".corrupt"
	if err := os.Rename(path, quarantine); err != nil {
		fmt.Fprintf(os.Stderr, "storage: %s is corrupt (%v); quarantine failed: %v\n", path, parseErr, err)
		return
	}
	fmt.Fprintf(os.Stderr, "storage: %s is corrupt (%v); moved to %s, continuing with defaults\n", path, parseErr, quarantine)
}

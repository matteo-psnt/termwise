package history

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// appendJSONL appends v to path as one JSON line, creating the file and its
// directory if needed. Callers hold their own lock; appends from separate
// processes rely on O_APPEND keeping each single write intact.
func appendJSONL(path string, v any) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	line, err := json.Marshal(v)
	if err != nil {
		_ = f.Close()
		return err
	}
	if _, err := f.Write(append(line, '\n')); err != nil {
		_ = f.Close()
		return err
	}
	return f.Close()
}

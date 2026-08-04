package sync

import (
	"os"
	"time"
)

type osEntry = struct {
	Name  string
	IsDir bool
}

func doMkdirAll(p string) error { return os.MkdirAll(p, 0o755) }
func doWriteFile(p, body string) error { return os.WriteFile(p, []byte(body), 0o644) }
func doAppend(p, body string) error {
	f, err := os.OpenFile(p, os.O_WRONLY|os.O_APPEND|os.O_CREATE, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.WriteString(body)
	return err
}
func doChtimes(p string, t time.Time) error { return os.Chtimes(p, t, t) }
func doReadDir(p string) ([]osEntry, error) {
	entries, err := os.ReadDir(p)
	if err != nil {
		return nil, err
	}
	out := make([]osEntry, len(entries))
	for i, e := range entries {
		out[i] = osEntry{Name: e.Name(), IsDir: e.IsDir()}
	}
	return out, nil
}
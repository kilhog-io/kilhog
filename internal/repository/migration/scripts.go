package migration

import (
	"embed"
	"fmt"
	"io/fs"
	"path"
	"sort"
	"strconv"
	"strings"
)

//go:embed migrations
var files embed.FS

type Script struct {
	Version int
	Name    string
	SQL     string
}

func loadScripts(dialect string, direction string) ([]Script, error) {
	root := path.Join("migrations", dialect)
	entries, err := fs.ReadDir(files, root)
	if err != nil {
		return nil, fmt.Errorf("read migrations for %s: %w", dialect, err)
	}

	suffix := "." + direction + ".sql"
	scripts := make([]Script, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), suffix) {
			continue
		}

		version, name, err := parseFileName(entry.Name(), suffix)
		if err != nil {
			return nil, err
		}

		content, err := fs.ReadFile(files, path.Join(root, entry.Name()))
		if err != nil {
			return nil, fmt.Errorf("read migration %s: %w", entry.Name(), err)
		}

		scripts = append(scripts, Script{
			Version: version,
			Name:    name,
			SQL:     string(content),
		})
	}

	sort.Slice(scripts, func(i, j int) bool {
		return scripts[i].Version < scripts[j].Version
	})

	return scripts, nil
}

func parseFileName(filename, suffix string) (int, string, error) {
	base := strings.TrimSuffix(filename, suffix)
	parts := strings.SplitN(base, "_", 2)
	if len(parts) != 2 {
		return 0, "", fmt.Errorf("invalid migration filename %q", filename)
	}

	version, err := strconv.Atoi(parts[0])
	if err != nil {
		return 0, "", fmt.Errorf("parse migration version in %q: %w", filename, err)
	}

	return version, parts[1], nil
}

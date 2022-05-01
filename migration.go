package main

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

type migration struct {
	version int
	name    string
	query   string
}

func parseMigrations(dir string) ([]migration, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("read migrations dir: %w", err)
	}

	migrationsByVersion := map[int]migration{}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		name := entry.Name()
		if !strings.HasSuffix(name, ".sql") {
			continue
		}

		version, err := parseMigrationName(name)
		if err != nil {
			return nil, err
		}

		if _, ok := migrationsByVersion[version]; ok {
			return nil, fmt.Errorf("two migrations for same version: %q, %q", name, migrationsByVersion[version].name)
		}

		query, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			return nil, fmt.Errorf("read migration file: %w", err)
		}

		migrationsByVersion[version] = migration{
			version: version,
			name:    name,
			query:   string(query),
		}
	}

	var migrations []migration
	for _, m := range migrationsByVersion {
		migrations = append(migrations, m)
	}

	sort.Slice(migrations, func(i, j int) bool { return migrations[i].version < migrations[j].version })

	return migrations, nil
}

var migrationNamePattern = regexp.MustCompile(`(\d+)_.*\.sql`)

func parseMigrationName(name string) (int, error) {
	match := migrationNamePattern.FindStringSubmatch(name)
	if match == nil {
		return 0, fmt.Errorf("migration name must begin with digits followed by underscore (`_`): %q", name)
	}

	n, err := strconv.Atoi(match[1])
	if err != nil {
		panic(err)
	}

	if n == 0 {
		return 0, fmt.Errorf("migration version must be nonzero: %q", name)
	}

	return n, nil
}

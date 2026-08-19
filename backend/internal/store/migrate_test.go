package store

import (
	"testing"
	"testing/fstest"
)

func TestMigrationVersionsAreSortedAndFiltered(t *testing.T) {
	fsys := fstest.MapFS{
		"0002_persistence.sql": {Data: []byte("select 2")},
		"0001_init.sql":        {Data: []byte("select 1")},
		"0010_later.sql":       {Data: []byte("select 10")},
		"README.md":            {Data: []byte("not a migration")},
		"embed.go":             {Data: []byte("package migrations")},
	}

	got, err := migrationVersions(fsys)
	if err != nil {
		t.Fatalf("migrationVersions: %v", err)
	}

	// Zero-padded prefixes are what make lexical order the apply order: with
	// "10_" instead of "0010_" this list would put 10 before 2.
	want := []string{"0001_init.sql", "0002_persistence.sql", "0010_later.sql"}
	if len(got) != len(want) {
		t.Fatalf("versions = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("versions = %v, want %v", got, want)
			break
		}
	}
}

func TestMigrationVersionsRejectsEmptySet(t *testing.T) {
	// An embed that silently matched nothing would make every deploy report
	// success while applying no schema at all.
	if _, err := migrationVersions(fstest.MapFS{"README.md": {}}); err == nil {
		t.Error("no error for a directory with no migrations")
	}
}

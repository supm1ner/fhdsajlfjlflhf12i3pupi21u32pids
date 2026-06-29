package database

import (
	"testing"
	"testing/fstest"

	"cotton-id/migrations"
)

func TestParseMigrationName(t *testing.T) {
	t.Parallel()
	cases := []struct {
		in       string
		wantVer  int
		wantName string
		wantErr  bool
	}{
		{"0001_init.up.sql", 1, "init", false},
		{"0042_add_thing.up.sql", 42, "add_thing", false},
		{"noversion.up.sql", 0, "", true},
		{"_init.up.sql", 0, "", true},
		{"abc_init.up.sql", 0, "", true},
	}
	for _, c := range cases {
		ver, name, err := parseMigrationName(c.in)
		if c.wantErr {
			if err == nil {
				t.Errorf("parseMigrationName(%q) expected error", c.in)
			}
			continue
		}
		if err != nil {
			t.Errorf("parseMigrationName(%q) unexpected error: %v", c.in, err)
			continue
		}
		if ver != c.wantVer || name != c.wantName {
			t.Errorf("parseMigrationName(%q) = (%d,%q), want (%d,%q)", c.in, ver, name, c.wantVer, c.wantName)
		}
	}
}

func TestLoadMigrationsSorts(t *testing.T) {
	t.Parallel()
	fsys := fstest.MapFS{
		"0002_second.up.sql":  {Data: []byte("SELECT 2;")},
		"0001_first.up.sql":   {Data: []byte("SELECT 1;")},
		"0001_first.down.sql": {Data: []byte("DROP;")}, // ignored
		"README.md":           {Data: []byte("nope")},  // ignored
	}
	migs, err := loadMigrations(fsys)
	if err != nil {
		t.Fatal(err)
	}
	if len(migs) != 2 {
		t.Fatalf("loaded %d migrations, want 2", len(migs))
	}
	if migs[0].version != 1 || migs[1].version != 2 {
		t.Fatalf("migrations not sorted ascending: %d then %d", migs[0].version, migs[1].version)
	}
	if migs[0].name != "first" {
		t.Errorf("name = %q, want first", migs[0].name)
	}
}

func TestLoadMigrationsRejectsDuplicateVersion(t *testing.T) {
	t.Parallel()
	fsys := fstest.MapFS{
		"0001_a.up.sql": {Data: []byte("SELECT 1;")},
		"0001_b.up.sql": {Data: []byte("SELECT 2;")},
	}
	if _, err := loadMigrations(fsys); err == nil {
		t.Fatal("expected duplicate-version error")
	}
}

// The real embedded migrations must parse and include version 1.
func TestEmbeddedMigrationsParse(t *testing.T) {
	t.Parallel()
	migs, err := loadMigrations(migrations.FS)
	if err != nil {
		t.Fatalf("embedded migrations failed to load: %v", err)
	}
	if len(migs) == 0 {
		t.Fatal("no embedded migrations found")
	}
	if migs[0].version != 1 {
		t.Errorf("first migration version = %d, want 1", migs[0].version)
	}
}

//go:build e2e

package e2e

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/sk-san/smart-food-manager/backend/internal/store"
	"github.com/sk-san/smart-food-manager/backend/migrations"
)

// TestMigrateIsIdempotent is the property the deploy pipeline depends on: the
// release command runs on every deploy, so a database that is already current
// must come back unchanged rather than failing on "type already exists".
func TestMigrateIsIdempotent(t *testing.T) {
	s := startSuite(t)

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	// The suite's database has the schema already, applied by whatever set it
	// up; the first call adopts or completes it.
	if _, err := store.Migrate(ctx, s.pool, migrations.FS, false); err != nil {
		t.Fatalf("first Migrate: %v", err)
	}

	applied, err := store.Migrate(ctx, s.pool, migrations.FS, false)
	if err != nil {
		t.Fatalf("second Migrate: %v", err)
	}
	if len(applied) != 0 {
		t.Errorf("second run applied %v, want nothing", applied)
	}

	pending, err := store.PendingMigrations(ctx, s.pool, migrations.FS)
	if err != nil {
		t.Fatalf("PendingMigrations: %v", err)
	}
	if len(pending) != 0 {
		t.Errorf("pending = %v after a full run, want none", pending)
	}
}

// TestMigrateRecordsEveryFile guards the other half: every migration in the
// repository must end up in schema_migrations, or a later deploy would try to
// re-run one against a schema that already has it.
func TestMigrateRecordsEveryFile(t *testing.T) {
	s := startSuite(t)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if _, err := store.Migrate(ctx, s.pool, migrations.FS, false); err != nil {
		t.Fatalf("Migrate: %v", err)
	}

	files, err := os.ReadDir("../migrations")
	if err != nil {
		t.Fatalf("read migrations dir: %v", err)
	}
	wantCount := 0
	for _, f := range files {
		if !f.IsDir() && len(f.Name()) > 4 && f.Name()[len(f.Name())-4:] == ".sql" {
			wantCount++
		}
	}

	var got int
	if err := s.pool.QueryRow(ctx, "SELECT count(*) FROM schema_migrations").Scan(&got); err != nil {
		t.Fatalf("count schema_migrations: %v", err)
	}
	if got != wantCount {
		t.Errorf("schema_migrations holds %d rows, want %d (one per .sql file)", got, wantCount)
	}
}

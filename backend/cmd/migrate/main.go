// Command migrate applies the embedded SQL migrations to the database named by
// DATABASE_URL and exits. It is the release step of a deploy: safe to run on
// every deploy, because a database that is already current is a no-op.
//
// Usage:
//
//	migrate                 # apply everything outstanding
//	migrate -baseline       # record migrations as applied without running them
//	migrate -dry-run        # report what would run, change nothing
//
// -baseline adopts a database whose schema predates this runner — the
// docker-compose development database, which applied the same files through
// docker-entrypoint-initdb.d before schema_migrations existed.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/sk-san/smart-food-manager/backend/internal/config"
	"github.com/sk-san/smart-food-manager/backend/internal/store"
	"github.com/sk-san/smart-food-manager/backend/migrations"
)

// runTimeout bounds the whole migration run; a release command that hangs
// holds up the deploy behind it.
const runTimeout = 5 * time.Minute

func main() {
	baseline := flag.Bool("baseline", false, "record pending migrations as applied without running them")
	dryRun := flag.Bool("dry-run", false, "report what would be applied and exit")
	flag.Parse()

	if err := run(*baseline, *dryRun); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(baseline, dryRun bool) error {
	cfg := config.Load()

	ctx, cancel := context.WithTimeout(context.Background(), runTimeout)
	defer cancel()

	pool, err := store.NewPool(ctx, cfg.DatabaseURL)
	if err != nil {
		return fmt.Errorf("migrate: connect: %w", err)
	}
	defer pool.Close()

	if dryRun {
		pending, err := store.PendingMigrations(ctx, pool, migrations.FS)
		if err != nil {
			return err
		}
		report("would apply", pending)
		return nil
	}

	applied, err := store.Migrate(ctx, pool, migrations.FS, baseline)
	if err != nil {
		return err
	}
	if baseline {
		report("recorded as applied (not run)", applied)
	} else {
		report("applied", applied)
	}
	return nil
}

// report prints the outcome, naming each migration so a deploy log says which
// change reached the database.
func report(what string, versions []string) {
	if len(versions) == 0 {
		fmt.Println("database is up to date; nothing to do")
		return
	}
	fmt.Printf("%s %d migration(s):\n", what, len(versions))
	for _, v := range versions {
		fmt.Println("  ", v)
	}
}

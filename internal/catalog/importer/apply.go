package importer

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/cosmofactory/recetas-backend/internal/catalog/catalogdb"
)

// importLockKey is the advisory lock that serialises imports.
const importLockKey int64 = 0x7265636574617301

// Beginner starts the import's transaction: a *pgxpool.Pool, or a pgx.Tx,
// whose Begin is a savepoint.
type Beginner interface {
	Begin(ctx context.Context) (pgx.Tx, error)
}

type Options struct {
	Source string // stored in content_versions
	DryRun bool   // report the changes, then roll them back
}

// Summary reports an import. Version is the content version afterwards; a
// dry run reports the unchanged one.
type Summary struct {
	Version   int64
	NoChanges bool
	Changes   map[string]Counts // by Entities
}

// Apply validates b and makes the stored catalog equal to it in one
// transaction: rows are matched by natural key, updated in place, created or
// deleted, and a content version recording opts.Source and the bundle's
// Checksum is added. The bundle is always compared with the stored rows, so
// drift is repaired; a bundle that changes nothing adds no version.
func Apply(ctx context.Context, db Beginner, b Bundle, opts Options) (Summary, error) {
	if err := Validate(b); err != nil {
		return Summary{}, err
	}
	tx, err := db.Begin(ctx)
	if err != nil {
		return Summary{}, fmt.Errorf("begin import: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	s, err := apply(ctx, catalogdb.New(tx), b, opts)
	if err != nil {
		return Summary{}, err
	}
	if opts.DryRun || s.NoChanges {
		return s, nil
	}
	if err := tx.Commit(ctx); err != nil {
		return Summary{}, fmt.Errorf("commit import: %w", err)
	}
	return s, nil
}

func apply(ctx context.Context, q *catalogdb.Queries, b Bundle, opts Options) (Summary, error) {
	if err := q.ImportLock(ctx, importLockKey); err != nil {
		return Summary{}, fmt.Errorf("lock import: %w", err)
	}
	latest, err := latestVersion(ctx, q)
	if err != nil {
		return Summary{}, err
	}
	cur, err := loadState(ctx, q)
	if err != nil {
		return Summary{}, fmt.Errorf("load catalog: %w", err)
	}
	p := diff(cur, b)
	s := Summary{Version: latest.ID, NoChanges: p.empty(), Changes: p.changes}
	if s.NoChanges {
		return s, nil
	}
	if err := newWriter(q, cur).execute(ctx, p); err != nil {
		return Summary{}, err
	}
	if !opts.DryRun {
		s.Version, err = recordVersion(ctx, q, opts.Source, Checksum(b))
	}
	return s, err
}

func recordVersion(ctx context.Context, q *catalogdb.Queries, source, checksum string) (int64, error) {
	id, err := q.ImportInsertContentVersion(ctx, catalogdb.ImportInsertContentVersionParams{Source: source, Checksum: checksum})
	if err != nil {
		return 0, fmt.Errorf("record content version: %w", err)
	}
	return id, nil
}

// latestVersion is the zero row before the first import.
func latestVersion(ctx context.Context, q *catalogdb.Queries) (catalogdb.ImportLatestContentVersionRow, error) {
	row, err := q.ImportLatestContentVersion(ctx)
	if errors.Is(err, pgx.ErrNoRows) {
		return catalogdb.ImportLatestContentVersionRow{}, nil
	}
	if err != nil {
		return row, fmt.Errorf("read content version: %w", err)
	}
	return row, nil
}

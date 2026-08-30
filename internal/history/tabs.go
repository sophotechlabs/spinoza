package history

import (
	"context"
	"fmt"
	"time"
)

type Tab struct {
	ID         string
	Context    string
	Kubeconfig string
	Seen       time.Time
	Color      int
	Label      string
	Grouping   string
	Reopen     bool
	Timeline   string
}

type Tabs struct {
	into *Store
}

func (s *Store) Tabs() *Tabs {
	return &Tabs{into: s}
}

func (t *Tabs) Remember(ctx context.Context, tab Tab) error {
	db := t.into.writer()
	if db == nil {
		return nil
	}
	_, err := db.ExecContext(ctx, upsertCluster,
		tab.ID, tab.Context, tab.Kubeconfig, tab.Seen.UTC().UnixMilli(), tab.Color,
		tab.Label, tab.Grouping, tab.Reopen)
	if err != nil {
		return fmt.Errorf("history: %w", err)
	}
	return nil
}

func (t *Tabs) Forget(ctx context.Context, id string) error {
	db := t.into.writer()
	if db == nil {
		return nil
	}
	_, err := db.ExecContext(ctx, deleteCluster, id)
	if err != nil {
		return fmt.Errorf("history: %w", err)
	}
	return nil
}

func (t *Tabs) Recolor(ctx context.Context, id string, color int) error {
	db := t.into.writer()
	if db == nil {
		return nil
	}
	_, err := db.ExecContext(ctx, recolorCluster, color, id)
	if err != nil {
		return fmt.Errorf("history: %w", err)
	}
	return nil
}

func (t *Tabs) Rename(ctx context.Context, id, label, grouping string) error {
	db := t.into.writer()
	if db == nil {
		return nil
	}
	_, err := db.ExecContext(ctx, renameCluster, label, grouping, id)
	if err != nil {
		return fmt.Errorf("history: %w", err)
	}
	return nil
}

func (t *Tabs) Reopening(ctx context.Context, id string, reopen bool) error {
	db := t.into.writer()
	if db == nil {
		return nil
	}
	_, err := db.ExecContext(ctx, reopenCluster, reopen, id)
	if err != nil {
		return fmt.Errorf("history: %w", err)
	}
	return nil
}

func (t *Tabs) Recording(ctx context.Context, id, kinds string) error {
	db := t.into.writer()
	if db == nil {
		return nil
	}
	_, err := db.ExecContext(ctx, recordCluster, kinds, id)
	if err != nil {
		return fmt.Errorf("history: %w", err)
	}
	return nil
}

func (t *Tabs) All(ctx context.Context) ([]Tab, error) {
	db := t.into.reader()
	if db == nil {
		return []Tab{}, nil
	}
	rows, err := db.QueryContext(ctx, selectClusters)
	if err != nil {
		return nil, fmt.Errorf("history: %w", err)
	}
	defer func() { _ = rows.Close() }()
	found := []Tab{}
	for rows.Next() {
		var tab Tab
		var seen int64
		scanErr := rows.Scan(&tab.ID, &tab.Context, &tab.Kubeconfig, &seen, &tab.Color,
			&tab.Label, &tab.Grouping, &tab.Reopen, &tab.Timeline)
		if scanErr != nil {
			return nil, fmt.Errorf("history: %w", scanErr)
		}
		tab.Seen = time.UnixMilli(seen).UTC()
		found = append(found, tab)
	}
	if rows.Err() != nil {
		return nil, fmt.Errorf("history: %w", rows.Err())
	}
	return found, nil
}

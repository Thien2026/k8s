package plugins

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	Console = "console"
	GHCR    = "ghcr"
	Rancher = "rancher"
	Harbor  = "harbor"
)

type Plugin struct {
	Name        string  `json:"name"`
	Label       string  `json:"label"`
	Description string  `json:"description"`
	Category    string  `json:"category"`
	Enabled     bool    `json:"enabled"`
	Core        bool    `json:"core"`
	Version     string  `json:"version,omitempty"`
	Bootstrap   string  `json:"bootstrap,omitempty"`
	InstalledAt *string `json:"installed_at,omitempty"`
	Ready         bool   `json:"ready"`
	ReadyHint     string `json:"ready_hint,omitempty"`
	NeedsBootstrap bool  `json:"needs_bootstrap"`
	ChartVersion  string `json:"chart_version,omitempty"`
	InstallCmd    string `json:"install_command,omitempty"`
	CheckCmd      string `json:"check_command,omitempty"`
	PrereqNote    string `json:"prereq_note,omitempty"`
}

type Store struct {
	db *pgxpool.Pool
}

func NewStore(db *pgxpool.Pool) *Store {
	return &Store{db: db}
}

func (s *Store) Enabled(ctx context.Context, name string) (bool, error) {
	var enabled bool
	err := s.db.QueryRow(ctx, `SELECT enabled FROM platform_plugins WHERE name=$1`, name).Scan(&enabled)
	if err != nil {
		if name == Console || name == GHCR {
			return name == GHCR, nil
		}
		return false, err
	}
	return enabled, nil
}

func (s *Store) List(ctx context.Context) ([]Plugin, error) {
	rows, err := s.db.Query(ctx, `
		SELECT name, label, description, category, enabled, core, version, bootstrap, installed_at
		FROM platform_plugins ORDER BY core DESC, category, name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var list []Plugin
	for rows.Next() {
		var p Plugin
		var installed *time.Time
		if err := rows.Scan(&p.Name, &p.Label, &p.Description, &p.Category, &p.Enabled, &p.Core, &p.Version, &p.Bootstrap, &installed); err != nil {
			return nil, err
		}
		if installed != nil {
			s := installed.UTC().Format(time.RFC3339)
			p.InstalledAt = &s
		}
		list = append(list, p)
	}
	if list == nil {
		list = []Plugin{}
	}
	return list, rows.Err()
}

func (s *Store) SetEnabled(ctx context.Context, name string, enabled bool) error {
	var core bool
	if err := s.db.QueryRow(ctx, `SELECT core FROM platform_plugins WHERE name=$1`, name).Scan(&core); err != nil {
		return err
	}
	if core && !enabled {
		return ErrCannotDisableCore
	}
	_, err := s.db.Exec(ctx, `
		UPDATE platform_plugins
		SET enabled=$2,
		    installed_at=CASE WHEN $2 AND installed_at IS NULL THEN now() ELSE installed_at END,
		    updated_at=now()
		WHERE name=$1`, name, enabled)
	return err
}

// ApplyEnvHints bật plugin lần đầu nếu engine đã cấu hình (VPS cũ sau migrate).
func (s *Store) ApplyEnvHints(ctx context.Context, rancherConfigured, harborConfigured bool) error {
	hints := map[string]bool{Rancher: rancherConfigured, Harbor: harborConfigured}
	for name, ok := range hints {
		if !ok {
			continue
		}
		_, err := s.db.Exec(ctx, `
			UPDATE platform_plugins
			SET enabled=true, installed_at=COALESCE(installed_at, now()), updated_at=now()
			WHERE name=$1 AND enabled=false AND installed_at IS NULL`, name)
		if err != nil {
			return err
		}
	}
	return nil
}

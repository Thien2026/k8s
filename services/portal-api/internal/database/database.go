package database

import (
	"context"
	"embed"

	"github.com/jackc/pgx/v5/pgxpool"
)

//go:embed migrations/*.sql
var migrations embed.FS

func Connect(ctx context.Context, url string) (*pgxpool.Pool, error) {
	return pgxpool.New(ctx, url)
}

func Migrate(ctx context.Context, db *pgxpool.Pool) error {
	sql, err := migrations.ReadFile("migrations/001_init.sql")
	if err != nil {
		return err
	}
	_, err = db.Exec(ctx, string(sql))
	return err
}

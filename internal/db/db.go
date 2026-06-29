package db

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

type WalletAddress struct {
	ID         string
	Address    string
	Type       string
	BusinessID string
	Active     bool
	CreatedAt  time.Time
}

type DB struct {
	pool *pgxpool.Pool
}

func Connect(ctx context.Context) (*DB, error) {
	url := os.Getenv("DATABASE_URL")
	if url == "" {
		url = "postgres://postgres:postgres@localhost:5432/postgres"
	}

	pool, err := pgxpool.New(ctx, url)
	if err != nil {
		return nil, fmt.Errorf("db: connect: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		return nil, fmt.Errorf("db: ping: %w", err)
	}
	return &DB{pool: pool}, nil
}

func (d *DB) RunMigration(ctx context.Context, sql string) error {
	_, err := d.pool.Exec(ctx, sql)
	return err
}

// SeedBusiness inserts the single MVP business row if it doesn't already exist.
// webhookURL and webhookSecret come from env vars so the DB row stays in sync.
func (d *DB) SeedBusiness(ctx context.Context, webhookURL, webhookSecret string) error {
	_, err := d.pool.Exec(ctx, `
		INSERT INTO businesses (business_id, name, webhook_url, webhook_secret, derivation_index, active)
		VALUES ('linq_mvp', 'Linq MVP', $1, $2, 0, true)
		ON CONFLICT (business_id) WHERE deleted_at IS NULL
		DO UPDATE SET webhook_url = $1, webhook_secret = $2, updated_at = NOW()
	`, webhookURL, webhookSecret)
	return err
}

// RegisterAddress inserts a wallet address tied to the linq_mvp business.
// type must equal the network_id from the indexer config (e.g. "sui_mainnet").
func (d *DB) RegisterAddress(ctx context.Context, address, networkType string) error {
	_, err := d.pool.Exec(ctx, `
		INSERT INTO wallet_addresses (address, type, business_id, active)
		VALUES ($1, $2, 'linq_mvp', true)
		ON CONFLICT (address, type) WHERE deleted_at IS NULL DO NOTHING
	`, address, networkType)
	if err != nil {
		return fmt.Errorf("db: register address: %w", err)
	}
	return nil
}

// LookupAddress returns the wallet record or nil if not found.
func (d *DB) LookupAddress(ctx context.Context, address string) (*WalletAddress, error) {
	var w WalletAddress
	err := d.pool.QueryRow(ctx, `
		SELECT id, address, type, business_id, active, created_at
		FROM wallet_addresses
		WHERE address = $1 AND active = true AND deleted_at IS NULL
		LIMIT 1
	`, address).Scan(&w.ID, &w.Address, &w.Type, &w.BusinessID, &w.Active, &w.CreatedAt)
	if err != nil {
		return nil, err
	}
	return &w, nil
}

func (d *DB) Close() {
	d.pool.Close()
}

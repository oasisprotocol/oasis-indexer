// Package postgres implements the target storage interface
// backed by PostgreSQL.
package postgres

import (
	"context"

	"github.com/jackc/pgx/v4"
	"github.com/jackc/pgx/v4/pgxpool"

	"github.com/oasislabs/oasis-indexer/log"
)

const (
	moduleName = "postgres"
)

var defaultMaxConns = int32(32)

// Client is a PostgreSQL client.
type Client struct {
	pool   *pgxpool.Pool
	logger *log.Logger
}

// NewClient creates a new PostgreSQL client.
func NewClient(connString string, l *log.Logger) (*Client, error) {
	config, err := pgxpool.ParseConfig(connString)
	if err != nil {
		return nil, err
	}
	config.MaxConns = defaultMaxConns

	pool, err := pgxpool.ConnectConfig(context.Background(), config)
	if err != nil {
		return nil, err
	}
	return &Client{
		pool:   pool,
		logger: l.WithModule(moduleName),
	}, nil
}

// SendBatch submits a new transaction batch to PostgreSQL.
func (c *Client) SendBatch(ctx context.Context, batch *pgx.Batch) error {
	if err := c.pool.BeginTxFunc(ctx, pgx.TxOptions{}, func(tx pgx.Tx) error {
		batchResults := tx.SendBatch(ctx, batch)
		defer batchResults.Close()
		for i := 0; i < batch.Len(); i++ {
			if _, err := batchResults.Exec(); err != nil {
				return err
			}
		}

		return nil
	}); err != nil {
		c.logger.Error("failed to execute tx batch",
			"error", err,
		)
		return err
	}

	return nil
}

// Query submits a new query to PostgreSQL.
func (c *Client) Query(ctx context.Context, sql string, args ...interface{}) (pgx.Rows, error) {
	rows, err := c.pool.Query(ctx, sql, args...)
	if err != nil {
		c.logger.Error("failed to query db",
			"error", err,
		)
		return nil, err
	}
	return rows, nil
}

// QueryRow submits a new query for a single row to PostgreSQL.
func (c *Client) QueryRow(ctx context.Context, sql string, args ...interface{}) pgx.Row {
	return c.pool.QueryRow(ctx, sql, args...)
}

// Shutdown shuts down the target storage client.
func (c *Client) Shutdown() {
	c.pool.Close()
}

// Name returns the name of the PostgreSQL client.
func (c *Client) Name() string {
	return moduleName
}

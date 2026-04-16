package db

import (
	"context"
	"fmt"

	"gorm.io/gorm"
)

func WithTx(ctx context.Context, database *gorm.DB, fn func(tx *gorm.DB) error) error {
	if database == nil {
		return fmt.Errorf("database is required")
	}
	if fn == nil {
		return fmt.Errorf("transaction callback is required")
	}
	return database.WithContext(ctx).Transaction(fn)
}

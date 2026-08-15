package repository

import (
	"fmt"
	"testing"
	"time"

	"github.com/anunay/wallet-service/internal/domain"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func newTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	dbName := fmt.Sprintf("file:memdb_%d_%d?mode=memory&cache=shared", time.Now().UnixNano(), time.Now().Nanosecond())
	db, err := gorm.Open(sqlite.Open(dbName), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("failed to open sqlite in-memory db: %v", err)
	}

	err = db.AutoMigrate(&domain.User{}, &domain.Wallet{}, &domain.Transfer{}, &domain.IdempotencyRecord{})
	if err != nil {
		t.Fatalf("failed to automigrate models: %v", err)
	}

	return db
}

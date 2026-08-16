package db

import (
	"database/sql"
	"fmt"

	"github.com/anunay/wallet-service/migrations"
	_ "github.com/lib/pq"
	"github.com/pressly/goose/v3"
	"go.uber.org/zap"
)

type gooseZapLogger struct {
	log *zap.Logger
}

func (l *gooseZapLogger) Printf(format string, v ...interface{}) {
	l.log.Info(fmt.Sprintf(format, v...))
}

func (l *gooseZapLogger) Fatalf(format string, v ...interface{}) {
	l.log.Fatal(fmt.Sprintf(format, v...))
}

func RunMigrations(dsn string, log *zap.Logger) error {
	goose.SetBaseFS(migrations.FS)
	goose.SetLogger(&gooseZapLogger{log: log})

	if err := goose.SetDialect("postgres"); err != nil {
		return fmt.Errorf("failed to set goose dialect: %w", err)
	}

	db, err := sql.Open("postgres", dsn)
	if err != nil {
		return fmt.Errorf("failed to open database connection for migrations: %w", err)
	}
	defer func() { _ = db.Close() }()

	if err := goose.Up(db, "."); err != nil {
		return fmt.Errorf("failed to apply migrations: %w", err)
	}

	log.Info("Database migrations applied successfully")
	return nil
}

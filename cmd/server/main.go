package main

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/anunay/wallet-service/config"
	"github.com/anunay/wallet-service/internal/db"
	"github.com/anunay/wallet-service/internal/logger"
	"github.com/anunay/wallet-service/internal/server"
	"go.uber.org/zap"
)

func main() {
	// 1. Load configuration
	cfg, err := config.LoadConfig("")
	if err != nil {
		fmt.Printf("Failed to load config: %v\n", err)
		os.Exit(1)
	}

	// 2. Setup Uber Zap logger
	log, err := logger.NewLogger(cfg.Log)
	if err != nil {
		fmt.Printf("Failed to initialize zap logger: %v\n", err)
		os.Exit(1)
	}
	defer func() { _ = log.Sync() }()

	log.Info("Starting Wallet Service...", zap.String("port", cfg.Server.Port), zap.String("log_level", cfg.Log.Level))

	// 3. Establish GORM Database Connection
	gormDB, err := db.NewGormDB(cfg.Database)
	if err != nil {
		log.Error("Failed to connect to database via GORM", zap.Error(err))
		os.Exit(1)
	}
	sqlDB, _ := gormDB.DB()
	if sqlDB != nil {
		defer func() { _ = sqlDB.Close() }()
	}
	log.Info("GORM database connection established", zap.String("host", cfg.Database.Host), zap.String("db", cfg.Database.Name))

	// 4. Run Migrations
	if err := db.RunMigrations(cfg.Database.DSN(), log); err != nil {
		log.Error("Failed to run database migrations", zap.Error(err))
		os.Exit(1)
	}

	// 5. Initialize Server (wires GORM repositories, services, handlers, routes & Fiber app)
	srv := server.InitializeServer(gormDB, cfg, log)

	// 6. Graceful Shutdown listener & Server Start
	shutdownChan := make(chan os.Signal, 1)
	signal.Notify(shutdownChan, os.Interrupt, syscall.SIGTERM)

	go func() {
		addr := ":" + cfg.Server.Port
		if err := srv.App.Listen(addr); err != nil {
			log.Info("Server stopped listening", zap.Error(err))
		}
	}()

	log.Info("Wallet Service running", zap.String("addr", ":"+cfg.Server.Port))

	<-shutdownChan
	log.Info("Shutting down server gracefully...")

	if err := srv.App.ShutdownWithTimeout(5 * time.Second); err != nil {
		log.Error("Error during server shutdown", zap.Error(err))
	} else {
		log.Info("Server shutdown complete")
	}
}

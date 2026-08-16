package e2e

// TestMain controls how the e2e suite is wired up. It supports two modes:
//
// # Local mode (default)
//
// An in-process Fiber server is booted against a dedicated Docker test
// database (wallet_postgres_test container, port 5433).
//
// Lifecycle:
//  1. Connect to Postgres and run all Goose migrations.
//  2. Boot the Fiber application on a random free port.
//  3. Run the test suite.
//  4. Gracefully shut down Fiber; the DB container is left for the Makefile
//     target to `docker compose down`.
//
// Override the DSN or the app port with environment variables:
//
//	E2E_DSN=postgres://... go test ./e2e/... -v
//	E2E_PORT=9090         go test ./e2e/... -v
//
// # Remote mode
//
// When BASE_URL is set in the environment before the test run, TestMain
// skips starting a local server and a local database entirely.  All tests
// fire real HTTP requests at the specified URL.
//
//	BASE_URL=https://<your-app>.onrender.com go test ./e2e/... -v -timeout 300s

import (
	"fmt"
	"log"
	"net"
	"os"
	"testing"
	"time"

	"github.com/anunay/wallet-service/config"
	"github.com/anunay/wallet-service/internal/db"
	"github.com/anunay/wallet-service/internal/server"
	"go.uber.org/zap"
)

// testServerPort holds the port chosen at startup so helpers can hit the
// right address even if E2E_PORT was not set.
var testServerPort string

func TestMain(m *testing.M) {
	// Remote mode: BASE_URL is already set — skip local infrastructure entirely.
	if remoteURL := os.Getenv("BASE_URL"); remoteURL != "" {
		log.Printf("[e2e] remote mode: running tests against %s", remoteURL)
		os.Exit(m.Run())
	}

	// Local mode: boot an in-process server and set BASE_URL for test helpers.
	port, cleanup := bootServer()
	testServerPort = port

	if err := os.Setenv("BASE_URL", "http://localhost:"+port); err != nil {
		log.Fatalf("setenv BASE_URL: %v", err)
	}

	code := m.Run()

	cleanup()
	os.Exit(code)
}

// bootServer starts an in-process wallet service and returns:
//   - the port it is listening on
//   - a cleanup func that shuts Fiber down gracefully
func bootServer() (port string, cleanup func()) {
	// ------------------------------------------------------------------ DSN
	dsn := os.Getenv("E2E_DSN")
	if dsn == "" {
		dsn = "postgres://wallet_test:wallet_test_secret@localhost:5433/walletdb_test?sslmode=disable"
	}

	// ------------------------------------------------------------------ Port
	port = os.Getenv("E2E_PORT")
	if port == "" {
		port = freePort()
	}

	// ------------------------------------------------------------------ Logger (silent during tests)
	zapCfg := zap.NewProductionConfig()
	zapCfg.Level = zap.NewAtomicLevelAt(zap.WarnLevel)
	logger, err := zapCfg.Build()
	if err != nil {
		log.Fatalf("build zap logger: %v", err)
	}

	// ------------------------------------------------------------------ Migrations
	log.Printf("[e2e] applying migrations via DSN %s", dsn)
	if err := db.RunMigrations(dsn, logger); err != nil {
		log.Fatalf("[e2e] migrations failed: %v", err)
	}
	log.Println("[e2e] migrations OK")

	// ------------------------------------------------------------------ App config
	cfg := &config.Config{
		Server: config.ServerConfig{
			Port:         port,
			ReadTimeout:  30 * time.Second,
			WriteTimeout: 30 * time.Second,
		},
		Database: config.DatabaseConfig{
			Host:         dbHost(dsn),
			Port:         dbPort(dsn),
			User:         "wallet_test",
			Password:     "wallet_test_secret",
			Name:         "walletdb_test",
			SSLMode:      "disable",
			MaxOpenConns: 10,
			MaxIdleConns: 5,
			MaxLifetime:  5 * time.Minute,
		},
		Auth: config.AuthConfig{
			JWTSecret:        "e2e-test-secret-do-not-use-in-prod",
			TokenExpiryHours: 24,
		},
		Log: config.LogConfig{Level: "warn", Format: "json"},
	}

	// ------------------------------------------------------------------ GORM connection
	gormDB, err := db.NewGormDB(cfg.Database)
	if err != nil {
		log.Fatalf("[e2e] gorm connect: %v", err)
	}

	// ------------------------------------------------------------------ Start Fiber
	appSrv := server.InitializeServer(gormDB, cfg, logger)
	go func() {
		if err := appSrv.App.Listen(":" + port); err != nil {
			// Fiber returns an error on clean shutdown — ignore it.
			log.Printf("[e2e] server stopped: %v", err)
		}
	}()

	// Wait until the port is accepting connections (max 5 s).
	waitForPort(port, 5*time.Second)
	log.Printf("[e2e] server ready on :%s", port)

	cleanup = func() {
		log.Println("[e2e] shutting down test server")
		if err := appSrv.App.ShutdownWithTimeout(5 * time.Second); err != nil {
			log.Printf("[e2e] shutdown error: %v", err)
		}
		if sqlDB, err := gormDB.DB(); err == nil {
			sqlDB.Close()
		}
	}
	return port, cleanup
}

// freePort asks the OS for an available TCP port.
func freePort() string {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		log.Fatalf("freePort: %v", err)
	}
	defer l.Close()
	return fmt.Sprintf("%d", l.Addr().(*net.TCPAddr).Port)
}

// waitForPort polls until the given port accepts a TCP connection or timeout.
func waitForPort(port string, timeout time.Duration) {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		conn, err := net.DialTimeout("tcp", "127.0.0.1:"+port, 200*time.Millisecond)
		if err == nil {
			conn.Close()
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	log.Fatalf("waitForPort: port %s not ready after %s", port, timeout)
}

// dbHost / dbPort parse a postgres:// DSN's host and port segments.
// They fall back to safe defaults if parsing fails.
func dbHost(dsn string) string {
	// postgres://user:pass@HOST:PORT/db?...
	host, _, err := splitHostPort(dsn)
	if err != nil {
		return "localhost"
	}
	return host
}

func dbPort(dsn string) string {
	_, port, err := splitHostPort(dsn)
	if err != nil {
		return "5433"
	}
	return port
}

func splitHostPort(dsn string) (host, port string, err error) {
	// Strip scheme and credentials: everything after "@" up to "/"
	at := -1
	for i, c := range dsn {
		if c == '@' {
			at = i
		}
	}
	if at < 0 {
		return "", "", fmt.Errorf("no @ in dsn")
	}
	rest := dsn[at+1:] // HOST:PORT/db?sslmode=...
	slash := -1
	for i, c := range rest {
		if c == '/' {
			slash = i
			break
		}
	}
	if slash >= 0 {
		rest = rest[:slash]
	}
	host, port, err = net.SplitHostPort(rest)
	return
}

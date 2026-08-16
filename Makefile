.PHONY: build-and-run test-unit test-e2e test-e2e-db-up test-e2e-db-down test-e2e-remote

# ── Dev ──────────────────────────────────────────────────────────────────────
build-and-run:
	docker compose down --remove-orphans
	docker compose up --build -d

run:
	docker compose down --remove-orphans
	docker compose up -d

run-clean:
	docker compose down --remove-orphans -v
	docker compose up --build -d

lint:
	golangci-lint run

# ── Unit tests ────────────────────────────────────────────────────────────────
test-unit:
	go test -race -v -count=1 $$(go list ./... | grep -v /e2e)

# ── E2E test database ────────────────────────────────────────────────────────
# Starts an ephemeral Postgres on port 5433 (no volume, tmpfs only).
test-e2e-db-up:
	docker compose -f docker-compose.test.yml up -d --wait
	@echo "✅ Test DB ready on localhost:5433"

# Tears down the test DB container and removes it.
test-e2e-db-down:
	docker compose -f docker-compose.test.yml down --remove-orphans

# ── Full e2e suite (local) ────────────────────────────────────────────────────
# Boots the test DB, runs all e2e tests in-process (no separate app container
# needed), then tears the DB back down.
#
#   make test-e2e                        – run all tests
#   make test-e2e RUN=TestConservation   – run one scenario
test-e2e: test-e2e-db-up
	go test ./e2e/... -v -count=1 -timeout 120s $(if $(RUN),-run $(RUN),)
	$(MAKE) test-e2e-db-down

# ── Remote e2e suite (Render / staging) ──────────────────────────────────────
# Runs the test suite against a live remote deployment.  No local DB or app
# server is started.  Requires BASE_URL to be set.
#
#   make test-e2e-remote BASE_URL=https://<your-app>.onrender.com
#   make test-e2e-remote BASE_URL=https://<your-app>.onrender.com RUN=TestConservation
#
#   make test-e2e-remote BASE_URL=http://localhost:8080
#
# Pre-warm the Render instance first to avoid cold-start timeouts:
#   curl -sf https://<your-app>.onrender.com/health
test-e2e-remote:
ifndef BASE_URL
	$(error BASE_URL is not set. Usage: make test-e2e-remote BASE_URL=https://<your-app>.onrender.com)
endif
	BASE_URL=$(BASE_URL) go test ./e2e/... -v -count=1 -timeout 300s $(if $(RUN),-run $(RUN),)

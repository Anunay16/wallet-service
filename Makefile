.PHONY: build-and-run test-unit test-e2e test-e2e-db-up test-e2e-db-down

# ── Dev ──────────────────────────────────────────────────────────────────────
build-and-run:
	docker compose down --remove-orphans
	docker compose up --build -d

# ── Unit tests ────────────────────────────────────────────────────────────────
test-unit:
	go test -race -v -count=1 ./internal/handler/... ./internal/service/... ./internal/repository/...

# ── E2E test database ────────────────────────────────────────────────────────
# Starts an ephemeral Postgres on port 5433 (no volume, tmpfs only).
test-e2e-db-up:
	docker compose -f docker-compose.test.yml up -d --wait
	@echo "✅ Test DB ready on localhost:5433"

# Tears down the test DB container and removes it.
test-e2e-db-down:
	docker compose -f docker-compose.test.yml down --remove-orphans

# ── Full e2e suite ───────────────────────────────────────────────────────────
# Boots the test DB, runs all e2e tests in-process (no separate app container
# needed), then tears the DB back down.
#
#   make test-e2e            – run all tests
#   make test-e2e RUN=TestConservation  – run one scenario
test-e2e: test-e2e-db-up
	go test ./e2e/... -v -count=1 -timeout 120s $(if $(RUN),-run $(RUN),)
	$(MAKE) test-e2e-db-down

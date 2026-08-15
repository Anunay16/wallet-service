# Testing Policy

1. **Do NOT automatically execute integration / smoke test scripts** (such as `./test_happy_path.sh` or `curl` test runs) without explicit user permission. Always ask the user before running integration test scripts.
2. **Running unit tests** using standard Go tools (`go test ./...`, `go vet ./...`, `go build ./...`) is allowed without asking.

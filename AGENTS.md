# Repository Guidelines

## Project Structure & Module Organization
`goclaw` is a Go monorepo organized by runtime domain. Core orchestration lives in `agent/` (runtime, tools, task SDK), transport adapters in `channels/`, model integrations in `providers/`, and API serving in `gateway/`. Shared runtime pieces are in `bus/`, `session/`, `memory/`, `cron/`, `config/`, and `internal/`.  
CLI entrypoints are in `cli/` and `cli/commands/`; main bootstrap is `main.go`.  
Tests are colocated with implementation files and follow `*_test.go` (for example, `agent/tools/registry_test.go`).

## Build, Test, and Development Commands
- `make build`: compile `goclaw` with version metadata.
- `make run`: run locally via `go run .`.
- `make test`: run all unit tests (`go test -v ./...`).
- `make test-race`: enable race detector (matches CI intent).
- `make test-coverage`: generate `coverage.out` and `coverage.html`.
- `make check`: run formatting check, vet, and lint.
- `make pre-commit`: run `fmt`, `vet`, `lint`, and `test` before pushing.
- `make ci`: full local CI (`deps`, checks, race, coverage).

## Coding Style & Naming Conventions
Use standard Go formatting and linting:
- Format with `make fmt` (`gofmt -s -w .`).
- Lint with `make lint` (`golangci-lint run ./...`).
- Validate static issues with `make vet`.

Naming conventions:
- Package names: short, lowercase, no underscores.
- File names: lowercase, use `_` only when it improves readability (`runtime_factory.go`).
- Tests: `TestXxx...` plus table-driven `t.Run(...)` when appropriate.

## Testing Guidelines
Primary framework is Go’s built-in `testing` package. Keep tests deterministic and near the target package.  
Minimum expectation for changes: run `make test`; for concurrency/path-sensitive changes, run `make test-race`; before PR, run `make ci`.  
Prefer behavior-focused test names such as `TestHandleRequestUnknownMethodReturnsMethodNotFound`.

## Commit & Pull Request Guidelines
Recent history follows Conventional Commit style (`feat:`, `fix:`, `refactor:`, `chore:`), optionally with scope (for example, `subagent:` or `providers:`). Keep subject lines imperative and specific.

PRs should include:
- concise summary of behavior changes,
- affected modules (for example, `agent/runtime`, `channels`),
- linked issue/task,
- test evidence (`make test` / `make ci` output),
- CLI/TUI screenshots or logs for UX-facing changes.

## Security & Configuration Tips
Do not commit secrets in `config.json` or provider credentials. Prefer local overrides and environment-based secrets.  
Skill/MCP overlays should live under `.agents/` (workspace/role/repo layers) and be reviewed like code.

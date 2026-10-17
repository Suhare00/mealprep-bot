# MealPrep Bot

Telegram meal-prep planner for couples, built with Go and PostgreSQL.

The project solves a recurring problem: deciding what to cook for several days ahead. The target workflow is meal preparation for two people, three meals per day, prepared for three days in advance.

## Product goal

Generate a practical three-day meal plan from a curated recipe database, while taking into account user preferences, previous meals, disabled recipes and later ratings. The service will also build a consolidated shopping list and support recipe editing directly from Telegram.

## Initial scenario

- 2 people
- 3 days
- 3 meals per day
- 18 prepared portions/containers in total
- recipes are stored as six-serving batches where appropriate

## Planned features

- recipe catalogue with ingredients and quantities
- recipe editing and soft deletion
- meal-plan generation
- prevention of recent repeats
- one-dish replacement
- consolidated shopping list
- cooking history and ratings
- Telegram interface
- PostgreSQL persistence
- Docker-based local development
- automated tests and CI

## Architecture direction

The Telegram layer is an adapter, not the business layer. Core meal-planning logic should remain independent from Telegram and PostgreSQL so it can be tested in isolation and reused by other interfaces later.

```text
Telegram
   |
   v
Telegram Adapter
   |
   v
Application Services
   |
   v
Domain
   |
   v
Repository Interfaces
   |
   v
PostgreSQL Adapter
```

## Tech stack

- Go
- PostgreSQL
- Telegram Bot API
- Docker / Docker Compose
- GitHub Actions
- golangci-lint
- integration tests with PostgreSQL

## Local development

Prerequisites:

- Go 1.27 or newer;
- Docker with Docker Compose;
- `golangci-lint` 2.15.0 for the `make lint` target;
- Make for the convenience targets.

Copy the example configuration and replace every `replace-with-...` placeholder with a local-only value:

```sh
cp .env.example .env
```

PowerShell equivalent:

```powershell
Copy-Item .env.example .env
```

The application reads environment variables directly; it does not load `.env` itself. To run the complete local stack, Docker Compose reads `.env` automatically:

```sh
docker compose up --build
```

PostgreSQL is exposed on `localhost:5432` for host-side tools. Inside Compose, the application uses the `postgres` service hostname rather than `localhost`.

To run the Go process directly, export the variables from `.env` in your shell first, then run:

```sh
go run ./cmd/bot
```

The current bootstrap validates `APP_ENV`, `DATABASE_URL`, `TELEGRAM_BOT_TOKEN`, and optional `LOG_LEVEL` at startup, emits structured JSON logs, and waits for `SIGINT` or `SIGTERM`. It intentionally does not connect to Telegram or PostgreSQL yet.

Run the same checks used by CI with:

```sh
make check
```

Individual targets are available for `fmt`, `fmt-check`, `vet`, `test`, `build`, `lint`, and `compose-config`.

## PostgreSQL migrations

Recipe catalogue migrations are explicit SQL files under `migrations/`. Durations are stored as whole seconds in `BIGINT` columns and ingredient quantities use PostgreSQL `NUMERIC`.

Start PostgreSQL and apply the current migration:

```sh
docker compose up -d postgres
make migrate-up
```

Reverse it in dependency-safe order with:

```sh
make migrate-down
```

The migration targets run `psql` inside the Compose PostgreSQL container and use its configured `POSTGRES_USER` and `POSTGRES_DB` values.

## PostgreSQL integration tests

Integration tests use the `integration` build tag and require a dedicated disposable PostgreSQL database. Ordinary `go test ./...` does not connect to PostgreSQL.

With the Compose database running, create the test database once:

```sh
docker compose exec -T postgres sh -c 'createdb -U "$POSTGRES_USER" mealprep_test'
```

Set `TEST_DATABASE_URL` to that database and run the suite:

```sh
export TEST_DATABASE_URL='postgres://mealprep:your-local-password@localhost:5432/mealprep_test?sslmode=disable'
make test-integration
```

PowerShell equivalent:

```powershell
$env:TEST_DATABASE_URL = 'postgres://mealprep:your-local-password@localhost:5432/mealprep_test?sslmode=disable'
make test-integration
```

The integration suite drops and recreates the recipe catalogue tables, so never point `TEST_DATABASE_URL` at production or a database containing data you need.

## Development approach

Development is issue-driven. Each meaningful feature should be implemented in a dedicated branch and merged through a pull request after tests and review pass.

AI-assisted coding may be used as development tooling, but architecture, requirements, review and acceptance decisions remain explicit and documented in the repository.

## Status

Early foundation stage.

See GitHub Issues for the current development plan.

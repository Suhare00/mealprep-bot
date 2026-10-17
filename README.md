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

The Go programs read environment variables directly; they do not load `.env` themselves. Docker Compose reads `.env` automatically and passes the configured values to its containers:

```sh
docker compose up --build
```

PostgreSQL is exposed on `localhost:5432` for host-side tools. Inside Compose, the application uses the `postgres` service hostname rather than `localhost`.

To run a Go process directly on the host, export the variables from `.env` in that shell first, then run:

```sh
go run ./cmd/bot
```

The bot validates `APP_ENV`, `DATABASE_URL`, `TELEGRAM_BOT_TOKEN`, and optional `LOG_LEVEL`, connects to PostgreSQL, verifies connectivity, checks that no Telegram webhook is active, and then starts long polling. `SIGINT` and `SIGTERM` cancel polling and close the database pool cleanly. Logs are structured JSON and runtime errors redact configured credentials.

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
make seed
```

`make seed` reads `DATABASE_URL` from the environment. When using the values from `.env`, export them into the current shell before running host-side Go commands. The seed CLI can also read another directory with `go run ./cmd/seed -dir /path/to/seed`.

Reverse it in dependency-safe order with:

```sh
make migrate-down
```

The migration targets run `psql` inside the Compose PostgreSQL container and use its configured `POSTGRES_USER` and `POSTGRES_DB` values.

## Canonical recipe seed

The `seed/` directory contains the canonical v0.1 catalogue:

- 90 recipes;
- 84 ingredients;
- 466 recipe-ingredient relations;
- 155 recipe-meal-type relations;
- 233 recipe-tag relations.

The importer parses and validates the complete dataset before opening its database transaction. It then applies the catalogue atomically. Repeating the exact import is safe: identical records are reported as already present and are not rewritten or duplicated. If a canonical recipe or ingredient ID already exists with different data, the command fails and rolls back the entire import instead of overwriting it. Recipes and ingredients whose IDs are not part of the canonical seed are left untouched.

`seed/provenance.csv` is retained as source documentation but is not imported because provenance is not yet part of the domain or database schema.

Run parser and validation tests without PostgreSQL:

```sh
go test ./internal/seed ./cmd/seed
```

The canonical PostgreSQL import, idempotency, conflict, preservation and rollback scenarios are part of the existing integration suite:

```sh
make test-integration
```

## Telegram catalogue

The Telegram MVP is read-only and supports private chats. It displays active recipes from PostgreSQL; it does not contain hardcoded recipe samples. Create a bot with [BotFather](https://t.me/BotFather), copy the token, and put it in your uncommitted `.env` as `TELEGRAM_BOT_TOKEN`. Never commit the real token.

For a clean local setup:

1. Copy `.env.example` to `.env` and replace the Telegram and PostgreSQL placeholders.
2. Start PostgreSQL:

   ```sh
   docker compose up -d postgres
   ```

3. Apply the migration:

   ```sh
   make migrate-up
   ```

4. Import the canonical catalogue. Because `make seed` runs Go on the host, first export `DATABASE_URL` from `.env` into the current shell, then run:

   ```sh
   make seed
   ```

5. Build and start the bot container; Compose reads `.env` automatically:

   ```sh
   docker compose up -d --build app
   ```

   Alternatively, export all application variables from `.env` into the host shell and run `go run ./cmd/bot`.

6. Open the bot in Telegram, send `/start`, and choose **Каталог рецептов**. `/recipes` opens the catalogue directly and `/help` shows the command list.

The bot uses Telegram `getUpdates` long polling. A webhook and long polling cannot be active at the same time; startup fails with a clear error when a webhook URL is configured. Run only one polling process for a bot token. Polling uses a bounded retry delay for transient failures and stops on context cancellation. Successfully processed update offsets are sent with the next polling request, providing best-effort at-least-once handling across process restarts.

The meal-planning button is an explicit placeholder. Meal plans, shopping lists, recipe editing, user history and ratings are not part of this MVP.

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

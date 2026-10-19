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

The bot validates `APP_ENV`, `DATABASE_URL`, `TELEGRAM_BOT_TOKEN`, optional `LOG_LEVEL`, and optional `MEDIA_ROOT`. For a host-side run, `MEDIA_ROOT` defaults to the absolute form of `./data/media`; Compose sets it to `/var/lib/mealprep/media`. Startup verifies that this directory is writable, connects to PostgreSQL, verifies connectivity, checks that no Telegram webhook is active, and then starts long polling. `SIGINT` and `SIGTERM` cancel polling and close the database pool cleanly. Logs are structured JSON and runtime errors redact configured credentials.

Run the same checks used by CI with:

```sh
make check
```

Individual targets are available for `fmt`, `fmt-check`, `vet`, `test`, `build`, `lint`, and `compose-config`.

## PostgreSQL migrations

Recipe catalogue migrations are explicit SQL files under `migrations/`. Durations are stored as whole seconds in `BIGINT` columns and ingredient quantities use PostgreSQL `NUMERIC`.

For a fresh database, start PostgreSQL and apply all three migrations once, in order:

```sh
docker compose up -d postgres
make migrate-up
make seed
```

`make migrate-up` is for a clean database: it applies the recipe catalogue, meal-plan, and recipe-enrichment schemas in order. Do not run it against a volume where migration 1 is already installed. The safe upgrade command for that case is documented below.

`make seed` reads `DATABASE_URL` from the environment. When using the values from `.env`, export them into the current shell before running host-side Go commands. The seed CLI can also read another directory with `go run ./cmd/seed -dir /path/to/seed`.

Reverse both migrations in dependency-safe order with:

```sh
make migrate-down
```

The migration targets run `psql` inside the Compose PostgreSQL container and use its configured `POSTGRES_USER` and `POSTGRES_DB` values. `make migrate-plan-up` / `make migrate-plan-down` apply only migration 2, while `make migrate-media-up` / `make migrate-media-down` apply only migration 3.

Migration 3 adds optional ordered recipe steps and image metadata. Steps use stable IDs, contiguous 1-based positions, UTF-8 text, and an optional image reference. Image bytes are never stored in PostgreSQL: the database stores stable media IDs, opaque storage keys, JPEG/PNG MIME types, decoded dimensions, and optional alt text/captions. Cover images, ordered additional images, and step images are explicit relations. Recipes with no steps are valid and mean “instructions not provided”; the canonical 90 recipes intentionally remain in that state.

The filesystem adapter accepts only content-verified JPEG and PNG files up to 10 MiB, with dimensions no larger than 10,000 pixels on either side and no more than 40 million decoded pixels. It generates opaque file names, rejects path-like keys, writes through a private temporary file, and atomically renames completed files. Stored files use owner-only permissions on Linux. PostgreSQL updates remain atomic, but PostgreSQL and the filesystem cannot share one transaction: an upload workflow must save the file first, delete it if metadata persistence fails, commit replacement metadata before deleting superseded files, and retain failed cleanup items for reconciliation. Admin upload/edit orchestration belongs to Issue #16.

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

## Telegram catalogue and meal planner

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

6. Open the bot in Telegram and verify:

   - `/start` opens the main menu;
   - `/recipes` opens the existing catalogue;
   - `/plan` opens the three-day planner;
   - **Сгенерировать план** creates breakfast, lunch and dinner;
   - **Список покупок** opens a choice between one consolidated list and separate breakfast, lunch and dinner lists;
   - the consolidated view combines matching ingredient IDs and units across all three recipes;
   - each per-dish view shows the actual recipe name, its six-serving yield and its original ingredient quantities;
   - replacing one meal leaves the other two unchanged;
   - after replacement, both shopping-list modes update for the new plan revision while the other two dish lists stay unchanged;
   - **Сгенерировать заново** replaces the complete plan.

The fixed MVP scenario is two people for three days. It selects three distinct six-serving recipes—one breakfast, one lunch and one dinner—so the three cooked batches provide 18 person-meal portions. The same dishes repeat over all three days by design. Eligible recipes must be active, support the relevant meal type and have at least three storage days. Recipes with a yield other than six servings are deliberately excluded until proportional batch scaling is introduced.

Shopping quantities come from the stored recipe relations. The consolidated view combines quantities only when both the canonical ingredient ID and unit are identical; unlike units are shown separately. Per-dish views preserve each recipe's own quantities. Shopping lists are derived from the current saved plan and are not persisted separately. Storage labels are curated catalogue data, not personalized food-safety guarantees.

The bot uses Telegram `getUpdates` long polling. A webhook and long polling cannot be active at the same time; startup fails with a clear error when a webhook URL is configured. Run only one polling process for a bot token. Polling uses a bounded retry delay for transient failures and stops on context cancellation. Successfully processed update offsets are sent with the next polling request, providing best-effort at-least-once handling across process restarts.

Callback acknowledgements are best effort. Telegram's expired/invalid callback-query response and unexpected acknowledgement transport failures are logged without callback IDs or credentials, but do not block the update offset or restart the process. Actual plan mutations and message delivery failures are still reported. Mutation callbacks carry the plan revision, and PostgreSQL compare-and-swap protection prevents old or replayed buttons from applying the same change twice.

Current plans and a small recent-selection window are stored per private chat in PostgreSQL, so plans survive a normal app/container restart. Recent recipes are avoided when alternatives exist. No preference profiles, allergy logic, nutrition, prices, cooking instructions, ratings, webhooks or group-chat planning are implemented.

## Safe upgrade from the 90-recipe database

If the existing Compose volume already contains migration 1 and the canonical 90 recipes, keep that volume and do not reseed or delete it:

```sh
docker compose stop app
git pull
docker compose up -d postgres
make migrate-plan-up
make migrate-media-up
docker compose build app
docker compose up -d app
```

Apply only migrations that are not already installed. `make migrate-plan-up` is needed only for a database that does not yet contain migration 2; `make migrate-media-up` applies only `000003_recipe_steps_media.up.sql`. Migration 3 is additive and does not rewrite recipe, seed, ingredient, or saved-plan rows. Do not use `docker compose down -v`, `make migrate-up`, or `make seed` for this upgrade.

For a database already upgraded through migration 2 (the normal post-PR #25 state), run these exact PowerShell commands from the repository root to apply only migration 3:

```powershell
docker compose stop app
git pull
docker compose up -d postgres
docker compose cp .\migrations\000003_recipe_steps_media.up.sql postgres:/tmp/000003_recipe_steps_media.up.sql
docker compose exec -T postgres sh -c 'psql -v ON_ERROR_STOP=1 -U "$POSTGRES_USER" -d "$POSTGRES_DB" -f /tmp/000003_recipe_steps_media.up.sql'
docker compose build app
docker compose up -d app
docker compose ps app postgres
```

These commands preserve the already-filled `.env`, existing `postgres-data` volume, all 90 imported recipes, and saved plans. The rebuilt app creates and mounts the separate `recipe-media` named volume at `/var/lib/mealprep/media`; Docker initializes it with ownership suitable for the non-root `app` user. Do not run `make migrate-up`, `make seed`, `docker compose down -v`, or any volume-removal/reset command during this upgrade.

After restart, send `/plan`, generate a plan, open the shopping list, replace one slot, then run:

```sh
docker compose restart app
```

Send `/plan` again and confirm that the same current plan remains. Pressing an old inline button should show the latest plan instead of applying an obsolete mutation. Logs should react immediately after restart and must not contain the bot token, complete API URL, database password or callback-query ID. Live Telegram behavior requires real credentials and is not exercised by automated tests.

The same persistence check in PowerShell is:

```powershell
docker compose restart app
docker compose ps app postgres
```

After both services report running, send `/plan` again and confirm that the plan created before the restart is unchanged.

## Recipe media backup and restore

Recipe media has two coordinated parts: metadata in PostgreSQL and files in the Compose `recipe-media` volume. Back up both while the app is stopped so they represent one logical point in time. These PowerShell commands avoid text redirection for the PostgreSQL custom-format dump:

```powershell
docker compose stop app
New-Item -ItemType Directory -Force .\backup | Out-Null
docker compose exec -T postgres sh -c 'pg_dump -Fc -U "$POSTGRES_USER" -d "$POSTGRES_DB" -f /tmp/mealprep.dump'
docker compose cp postgres:/tmp/mealprep.dump .\backup\mealprep.dump
$backupDir = (Resolve-Path .\backup).Path
docker compose run --rm --no-deps -v "${backupDir}:/backup" --entrypoint sh app -c 'tar -C /var/lib/mealprep/media -czf /backup/recipe-media.tar.gz .'
docker compose up -d app
```

Restore only into the intended database and media volume, with the app stopped. `pg_restore --clean` replaces objects in that target database, so verify the Compose project and backup first:

```powershell
docker compose stop app
docker compose cp .\backup\mealprep.dump postgres:/tmp/mealprep.dump
docker compose exec -T postgres sh -c 'pg_restore --clean --if-exists -U "$POSTGRES_USER" -d "$POSTGRES_DB" /tmp/mealprep.dump'
$backupDir = (Resolve-Path .\backup).Path
docker compose run --rm --no-deps -v "${backupDir}:/backup:ro" --entrypoint sh app -c 'find /var/lib/mealprep/media -mindepth 1 -maxdepth 1 -type f -delete && tar -C /var/lib/mealprep/media -xzf /backup/recipe-media.tar.gz'
docker compose up -d app
```

The concrete media volume name is Compose-project-specific; `docker volume ls` shows it, normally ending in `_recipe-media`. Never remove either the PostgreSQL or media volume unless a verified backup exists. Missing files are treated as broken data, not as an empty-instructions state. There is currently no Telegram photo rendering (#17) or authenticated admin upload/edit UI (#16), and no instructions or images are invented for canonical seed recipes.

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

The integration suite drops and recreates the recipe, recipe-enrichment, and meal-plan tables, so never point `TEST_DATABASE_URL` at production or a database containing data you need.

## Development approach

Development is issue-driven. Each meaningful feature should be implemented in a dedicated branch and merged through a pull request after tests and review pass.

AI-assisted coding may be used as development tooling, but architecture, requirements, review and acceptance decisions remain explicit and documented in the repository.

## Status

Early foundation stage.

See GitHub Issues for the current development plan.

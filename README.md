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

## Development approach

Development is issue-driven. Each meaningful feature should be implemented in a dedicated branch and merged through a pull request after tests and review pass.

AI-assisted coding may be used as development tooling, but architecture, requirements, review and acceptance decisions remain explicit and documented in the repository.

## Status

Early foundation stage.

See GitHub Issues for the current development plan.

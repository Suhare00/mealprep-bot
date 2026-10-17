# AGENTS.md

This file defines repository-wide rules for AI coding agents and human contributors.

## Scope

Build a production-oriented Go backend for a Telegram meal-prep planner. Keep the implementation simple, explicit and testable. Do not add abstraction without a current need.

## Architecture rules

- Business logic must not depend on Telegram.
- Domain and application services must not depend on PostgreSQL-specific types.
- External systems belong behind adapters/interfaces where isolation is useful.
- Prefer clear package boundaries over deep layering.
- Avoid circular dependencies.
- Do not introduce global mutable state.

## Go rules

- Write idiomatic Go.
- Run `gofmt` on modified Go files.
- Use `context.Context` for request-scoped and I/O operations.
- Never replace an incoming request context with `context.Background()` in a request flow.
- Wrap errors with `%w` when preserving the cause is useful.
- Do not panic for expected runtime failures.
- Keep exported APIs small.
- Prefer standard library packages unless a dependency has a concrete benefit.
- Do not add a dependency without explaining why in the PR description.

## Database rules

- PostgreSQL is the persistence layer.
- Every schema change requires a reversible migration where practical.
- Use parameterized SQL only.
- Recipe deletion is soft deletion by default; history must remain valid.
- Keep ingredient quantities on the recipe-ingredient relation rather than the ingredient entity.

## Testing rules

- New business logic requires tests.
- Bugs should receive regression tests when practical.
- Repository/database behavior should be covered by integration tests when persistence is introduced.
- Tests must be deterministic and independent from Telegram network access.

## Configuration and secrets

- Never commit tokens, passwords or `.env` files containing secrets.
- Document configuration through `.env.example`.
- Fail fast on invalid required configuration at startup.

## Git and task discipline

- Work on one GitHub Issue at a time unless the issue explicitly groups related work.
- Do not modify unrelated files.
- Use focused commits with Conventional Commit-style messages where practical.
- Branch names should describe the issue, for example `feat/recipe-storage` or `chore/project-bootstrap`.
- Every non-trivial change goes through a pull request.

## Pull request expectations

A PR should state:

- what changed;
- why it changed;
- how it was tested;
- database/configuration changes;
- known limitations or follow-up work.

## Definition of Done

Before marking work complete:

1. implementation satisfies the linked Issue acceptance criteria;
2. formatting passes;
3. `go vet ./...` passes once Go code exists;
4. `go test ./...` passes once Go code exists;
5. linter passes once configured;
6. no secrets or generated binaries are committed;
7. documentation is updated if behavior or architecture changed.

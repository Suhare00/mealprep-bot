# Recipe seed dataset

This directory contains the canonical v0.1 seed dataset for the recipe catalogue.

The files are derived from the curated workbook `meal_prep_recipe_database_v0_1.xlsx`
and are normalized to the domain model currently implemented by the application.

## Imported files

- `recipes.csv` — scalar recipe fields.
- `ingredients.csv` — stable ingredient IDs and names.
- `recipe_ingredients.csv` — recipe-to-ingredient quantities and units.
- `recipe_meal_types.csv` — one row per recipe/meal-slot relation.
- `recipe_tags.csv` — one row per recipe/tag relation.

## Provenance

`provenance.csv` keeps source-name/source-URL metadata from the curated dataset for
future use. It is documentation-only in the current schema and must not be imported
until source metadata becomes part of the domain and database model.

## Canonical mappings from the original workbook

Meal types:
- `Завтрак` -> `breakfast`
- `Обед` -> `lunch`
- `Обед/ужин` -> two rows: `lunch` and `dinner`

Difficulty:
- `Легко` -> `easy`
- `Средне` -> `medium`

Budget:
- `₽` -> `low`
- `₽₽` -> `medium`
- `₽₽₽` -> `high`

Durations are stored in whole seconds. All recipes in v0.1 use six servings.

## Dataset size

- 90 recipes
- 84 ingredients
- 466 recipe-ingredient relations
- 155 recipe-meal-type relations
- 233 recipe-tag relations

The repository importer must validate all files before mutating the database and must
be safe to run repeatedly.

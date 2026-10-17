CREATE TABLE recipes (
    id TEXT PRIMARY KEY CHECK (btrim(id) <> ''),
    name TEXT NOT NULL CHECK (btrim(name) <> ''),
    category TEXT NOT NULL CHECK (btrim(category) <> ''),
    servings INTEGER NOT NULL CHECK (servings > 0),
    preparation_seconds BIGINT NOT NULL CHECK (preparation_seconds >= 0),
    cooking_seconds BIGINT NOT NULL CHECK (cooking_seconds >= 0),
    storage_days INTEGER NOT NULL CHECK (storage_days > 0),
    freezer_friendly BOOLEAN NOT NULL,
    difficulty TEXT NOT NULL CHECK (difficulty IN ('easy', 'medium')),
    budget_level TEXT NOT NULL CHECK (budget_level IN ('low', 'medium', 'high')),
    reheat_score SMALLINT NOT NULL CHECK (reheat_score BETWEEN 1 AND 5),
    batch_cooking_score SMALLINT NOT NULL CHECK (batch_cooking_score BETWEEN 1 AND 5),
    state TEXT NOT NULL CHECK (state IN ('active', 'inactive'))
);

CREATE TABLE ingredients (
    id TEXT PRIMARY KEY CHECK (btrim(id) <> ''),
    name TEXT NOT NULL CHECK (btrim(name) <> '')
);

CREATE TABLE recipe_meal_types (
    recipe_id TEXT NOT NULL REFERENCES recipes(id) ON DELETE CASCADE,
    meal_type TEXT NOT NULL CHECK (meal_type IN ('breakfast', 'lunch', 'dinner')),
    PRIMARY KEY (recipe_id, meal_type)
);

CREATE TABLE recipe_tags (
    recipe_id TEXT NOT NULL REFERENCES recipes(id) ON DELETE CASCADE,
    tag TEXT NOT NULL CHECK (btrim(tag) <> ''),
    PRIMARY KEY (recipe_id, tag)
);

CREATE TABLE recipe_ingredients (
    recipe_id TEXT NOT NULL REFERENCES recipes(id) ON DELETE CASCADE,
    ingredient_id TEXT NOT NULL REFERENCES ingredients(id) ON DELETE RESTRICT,
    quantity NUMERIC NOT NULL CHECK (quantity > 0),
    unit TEXT NOT NULL CHECK (btrim(unit) <> ''),
    PRIMARY KEY (recipe_id, ingredient_id)
);

CREATE INDEX recipes_active_id_idx ON recipes (id) WHERE state = 'active';

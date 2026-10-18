CREATE TABLE meal_plans (
    chat_id BIGINT PRIMARY KEY,
    breakfast_recipe_id TEXT NOT NULL REFERENCES recipes(id) ON DELETE RESTRICT,
    lunch_recipe_id TEXT NOT NULL REFERENCES recipes(id) ON DELETE RESTRICT,
    dinner_recipe_id TEXT NOT NULL REFERENCES recipes(id) ON DELETE RESTRICT,
    revision BIGINT NOT NULL CHECK (revision > 0),
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,
    CHECK (created_at <= updated_at),
    CHECK (breakfast_recipe_id <> lunch_recipe_id),
    CHECK (breakfast_recipe_id <> dinner_recipe_id),
    CHECK (lunch_recipe_id <> dinner_recipe_id)
);

CREATE TABLE meal_plan_recent_selections (
    chat_id BIGINT NOT NULL REFERENCES meal_plans(chat_id) ON DELETE CASCADE,
    sequence BIGINT GENERATED ALWAYS AS IDENTITY,
    recipe_id TEXT NOT NULL REFERENCES recipes(id) ON DELETE RESTRICT,
    selected_at TIMESTAMPTZ NOT NULL,
    PRIMARY KEY (chat_id, sequence)
);

CREATE INDEX meal_plan_recent_chat_sequence_idx
    ON meal_plan_recent_selections (chat_id, sequence DESC);

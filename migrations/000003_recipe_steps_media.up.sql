CREATE TABLE recipe_media (
    id TEXT PRIMARY KEY CHECK (btrim(id) <> ''),
    recipe_id TEXT NOT NULL REFERENCES recipes(id) ON DELETE CASCADE,
    storage_key TEXT NOT NULL UNIQUE CHECK (
        storage_key = btrim(storage_key)
        AND length(storage_key) BETWEEN 1 AND 255
        AND strpos(storage_key, '/') = 0
        AND strpos(storage_key, chr(92)) = 0
        AND strpos(storage_key, '..') = 0
        AND left(storage_key, 1) <> '.'
    ),
    mime_type TEXT NOT NULL CHECK (mime_type IN ('image/jpeg', 'image/png')),
    width INTEGER NOT NULL CHECK (width > 0 AND width <= 10000),
    height INTEGER NOT NULL CHECK (height > 0 AND height <= 10000),
    alt_text TEXT CHECK (alt_text IS NULL OR btrim(alt_text) <> ''),
    caption TEXT CHECK (caption IS NULL OR btrim(caption) <> ''),
    UNIQUE (recipe_id, id),
    CHECK (width::BIGINT * height::BIGINT <= 40000000)
);

CREATE TABLE recipe_cover_images (
    recipe_id TEXT PRIMARY KEY REFERENCES recipes(id) ON DELETE CASCADE,
    media_id TEXT NOT NULL UNIQUE,
    FOREIGN KEY (recipe_id, media_id)
        REFERENCES recipe_media(recipe_id, id) ON DELETE CASCADE
);

CREATE TABLE recipe_images (
    recipe_id TEXT NOT NULL REFERENCES recipes(id) ON DELETE CASCADE,
    media_id TEXT NOT NULL UNIQUE,
    position INTEGER NOT NULL CHECK (position > 0),
    PRIMARY KEY (recipe_id, position),
    FOREIGN KEY (recipe_id, media_id)
        REFERENCES recipe_media(recipe_id, id) ON DELETE CASCADE
);

CREATE TABLE recipe_steps (
    id TEXT PRIMARY KEY CHECK (btrim(id) <> ''),
    recipe_id TEXT NOT NULL REFERENCES recipes(id) ON DELETE CASCADE,
    position INTEGER NOT NULL CHECK (position > 0),
    text TEXT NOT NULL CHECK (btrim(text) <> ''),
    image_media_id TEXT UNIQUE,
    UNIQUE (recipe_id, position),
    UNIQUE (recipe_id, id),
    FOREIGN KEY (recipe_id, image_media_id)
        REFERENCES recipe_media(recipe_id, id) ON DELETE RESTRICT
);


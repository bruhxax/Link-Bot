CREATE TABLE theme_preset (
    id BIGSERIAL PRIMARY KEY,
    name TEXT NOT NULL,
    appearance JSONB NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT theme_preset_name_length CHECK (char_length(name) BETWEEN 1 AND 48)
);

CREATE UNIQUE INDEX theme_preset_name_unique ON theme_preset (lower(name));

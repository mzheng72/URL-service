CREATE TABLE urls (
    id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    short_code VARCHAR(8) NOT NULL UNIQUE,
    original_url TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT short_code_length CHECK (char_length(short_code) = 8),
    CONSTRAINT original_url_not_empty CHECK (char_length(original_url) > 0)
);
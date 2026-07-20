ALTER TABLE review
    ADD COLUMN IF NOT EXISTS deleted_at TIMESTAMPTZ NULL;

CREATE INDEX IF NOT EXISTS review_deleted_at_idx ON review(deleted_at);

DROP INDEX IF EXISTS review_deleted_at_idx;

ALTER TABLE review
    DROP COLUMN IF EXISTS deleted_at;

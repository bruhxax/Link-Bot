DROP INDEX IF EXISTS customer_blocked_idx;
DROP INDEX IF EXISTS customer_admin_search_idx;

ALTER TABLE customer
    DROP COLUMN IF EXISTS blocked_at,
    DROP COLUMN IF EXISTS is_blocked;

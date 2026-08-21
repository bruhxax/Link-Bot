DROP INDEX IF EXISTS purchase_free_plan_claim_idx;

ALTER TABLE purchase
    DROP COLUMN IF EXISTS free_plan_one_time,
    DROP COLUMN IF EXISTS is_free_plan;

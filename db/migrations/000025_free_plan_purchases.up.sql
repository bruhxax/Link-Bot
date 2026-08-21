ALTER TABLE purchase
    ADD COLUMN IF NOT EXISTS is_free_plan BOOLEAN NOT NULL DEFAULT FALSE,
    ADD COLUMN IF NOT EXISTS free_plan_one_time BOOLEAN NOT NULL DEFAULT FALSE;

CREATE INDEX IF NOT EXISTS purchase_free_plan_claim_idx
    ON purchase (customer_id, plan_id, status)
    WHERE is_free_plan = TRUE;

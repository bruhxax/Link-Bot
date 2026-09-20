ALTER TABLE customer
    ADD COLUMN IF NOT EXISTS blocked_reason TEXT;

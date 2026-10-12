-- Manual panel limits replace earlier add-on entitlements, without deleting
-- their payment history. Future purchases start from the actual panel limits.
ALTER TABLE purchase ADD COLUMN device_entitlement_superseded_at TIMESTAMPTZ;
ALTER TABLE purchase ADD COLUMN traffic_entitlement_superseded_at TIMESTAMPTZ;
-- A new invoice must not be mistaken for a retry merely because a manual
-- panel limit happens to equal that invoice's old expected target.
ALTER TABLE purchase ADD COLUMN entitlement_prepared_at TIMESTAMPTZ;

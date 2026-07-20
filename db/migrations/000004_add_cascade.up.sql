BEGIN;
ALTER TABLE purchase DROP CONSTRAINT IF EXISTS purchase_customer_id_fkey;
ALTER TABLE referral DROP CONSTRAINT IF EXISTS referral_referrer_id_fkey;
ALTER TABLE referral DROP CONSTRAINT IF EXISTS referral_referee_id_fkey;

ALTER TABLE purchase
    ADD CONSTRAINT purchase_customer_id_fkey
        FOREIGN KEY (customer_id)
            REFERENCES customer(id)
            ON DELETE CASCADE
            DEFERRABLE INITIALLY DEFERRED;

ALTER TABLE referral
    ADD CONSTRAINT referral_referrer_id_fkey
        FOREIGN KEY (referrer_id)
            REFERENCES customer(telegram_id)
            ON DELETE CASCADE;

ALTER TABLE referral
    ADD CONSTRAINT referral_referee_id_fkey
        FOREIGN KEY (referee_id)
            REFERENCES customer(telegram_id)
            ON DELETE CASCADE;
COMMIT;
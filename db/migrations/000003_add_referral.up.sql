BEGIN;

ALTER TABLE purchase
DROP CONSTRAINT IF EXISTS purchase_customer_id_fkey;
ALTER TABLE purchase
    ADD CONSTRAINT purchase_customer_id_fkey
        FOREIGN KEY (customer_id)
            REFERENCES customer(id)
            DEFERRABLE INITIALLY DEFERRED;

SET CONSTRAINTS purchase_customer_id_fkey DEFERRED;

CREATE TEMP TABLE tmp_duplicates ON COMMIT DROP AS
SELECT telegram_id
FROM customer
GROUP BY telegram_id
HAVING COUNT(*) > 1;

CREATE TEMP TABLE tmp_keepers ON COMMIT DROP AS
SELECT DISTINCT ON (c.telegram_id)
    c.telegram_id,
    c.id AS keep_id
FROM customer c
    JOIN tmp_duplicates d ON c.telegram_id = d.telegram_id
ORDER BY c.telegram_id, c.expire_at DESC, c.id ASC;

CREATE TEMP TABLE tmp_others ON COMMIT DROP AS
SELECT c.id AS old_id,
       k.keep_id
FROM customer c
         JOIN tmp_keepers k ON c.telegram_id = k.telegram_id
WHERE c.id <> k.keep_id;

UPDATE purchase p
SET customer_id = o.keep_id
    FROM tmp_others o
WHERE p.customer_id = o.old_id;

DELETE FROM customer c
    USING tmp_others o
WHERE c.id = o.old_id;

COMMIT;


ALTER TABLE customer DROP CONSTRAINT IF EXISTS customer_telegram_id_unique;

ALTER TABLE customer ADD CONSTRAINT customer_telegram_id_unique UNIQUE (telegram_id);


CREATE TABLE IF NOT EXISTS referral (
                          id             BIGSERIAL PRIMARY KEY,
                          referrer_id    BIGINT NOT NULL REFERENCES customer(telegram_id),
                          referee_id     BIGINT NOT NULL REFERENCES customer(telegram_id),
                          used_at        TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
                          bonus_granted  BOOLEAN    NOT NULL DEFAULT FALSE
);

CREATE INDEX IF NOT EXISTS idx_referral_referrer_id ON referral USING hash (referrer_id);
CREATE INDEX IF NOT EXISTS idx_referral_referee_id  ON referral USING hash (referee_id);



ALTER TABLE promo_code DROP CONSTRAINT promo_code_personal_limit;
ALTER TABLE promo_code DROP COLUMN review_id, DROP COLUMN owner_customer_id;
DROP TABLE review_reward_claim;
ALTER TABLE review DROP COLUMN reward_settings;

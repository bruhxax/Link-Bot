CREATE TABLE referral_reward (
    id BIGSERIAL PRIMARY KEY,
    referral_id BIGINT NOT NULL REFERENCES referral(id) ON DELETE CASCADE,
    event_type VARCHAR(20) NOT NULL CHECK (event_type IN ('trial', 'purchase')),
    purchase_id BIGINT REFERENCES purchase(id) ON DELETE SET NULL,
    reward_days INTEGER NOT NULL DEFAULT 0 CHECK (reward_days >= 0),
    reward_traffic_bytes BIGINT NOT NULL DEFAULT 0 CHECK (reward_traffic_bytes >= 0),
    reward_balance_cents BIGINT NOT NULL DEFAULT 0 CHECK (reward_balance_cents >= 0),
    status VARCHAR(20) NOT NULL DEFAULT 'processing' CHECK (status IN ('processing', 'granted', 'failed')),
    error_message TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    granted_at TIMESTAMPTZ
);

CREATE UNIQUE INDEX referral_reward_trial_once_idx
    ON referral_reward (referral_id, event_type)
    WHERE event_type = 'trial';

CREATE UNIQUE INDEX referral_reward_purchase_once_idx
    ON referral_reward (purchase_id)
    WHERE purchase_id IS NOT NULL;

CREATE TABLE balance_transaction (
    id BIGSERIAL PRIMARY KEY,
    customer_id BIGINT NOT NULL REFERENCES customer(id) ON DELETE CASCADE,
    amount_cents BIGINT NOT NULL CHECK (amount_cents <> 0),
    balance_after_cents BIGINT NOT NULL CHECK (balance_after_cents >= 0),
    kind VARCHAR(40) NOT NULL,
    reference_key VARCHAR(160) NOT NULL UNIQUE,
    description TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX balance_transaction_customer_idx
    ON balance_transaction (customer_id, created_at DESC, id DESC);

CREATE TABLE balance_withdrawal (
    id BIGSERIAL PRIMARY KEY,
    customer_id BIGINT NOT NULL REFERENCES customer(id) ON DELETE CASCADE,
    amount_cents BIGINT NOT NULL CHECK (amount_cents > 0),
    payout_details TEXT NOT NULL,
    status VARCHAR(20) NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'approved', 'rejected')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    resolved_at TIMESTAMPTZ,
    resolved_by BIGINT
);

CREATE INDEX balance_withdrawal_customer_idx
    ON balance_withdrawal (customer_id, created_at DESC, id DESC);

CREATE INDEX balance_withdrawal_pending_idx
    ON balance_withdrawal (created_at ASC, id ASC)
    WHERE status = 'pending';

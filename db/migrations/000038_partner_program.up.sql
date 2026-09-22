CREATE TABLE partner_application (
    id BIGSERIAL PRIMARY KEY,
    customer_id BIGINT NOT NULL UNIQUE REFERENCES customer(id) ON DELETE CASCADE,
    resource_url TEXT NOT NULL,
    requested_percent SMALLINT NOT NULL CHECK (requested_percent BETWEEN 0 AND 100),
    expected_monthly_users INTEGER NOT NULL CHECK (expected_monthly_users >= 0),
    status VARCHAR(16) NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'approved', 'rejected')),
    reviewed_at TIMESTAMP WITH TIME ZONE,
    reviewed_by BIGINT REFERENCES customer(id) ON DELETE SET NULL,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW()
);

CREATE TABLE partner (
    id BIGSERIAL PRIMARY KEY,
    customer_id BIGINT NOT NULL UNIQUE REFERENCES customer(id) ON DELETE CASCADE,
    application_id BIGINT UNIQUE REFERENCES partner_application(id) ON DELETE SET NULL,
    code VARCHAR(32) NOT NULL UNIQUE,
    commission_percent SMALLINT NOT NULL CHECK (commission_percent BETWEEN 0 AND 100),
    is_active BOOLEAN NOT NULL DEFAULT TRUE,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW()
);

CREATE TABLE partner_referral (
    id BIGSERIAL PRIMARY KEY,
    partner_id BIGINT NOT NULL REFERENCES partner(id) ON DELETE CASCADE,
    customer_id BIGINT NOT NULL UNIQUE REFERENCES customer(id) ON DELETE CASCADE,
    attached_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW()
);

CREATE TABLE partner_commission (
    id BIGSERIAL PRIMARY KEY,
    partner_id BIGINT NOT NULL REFERENCES partner(id) ON DELETE CASCADE,
    customer_id BIGINT NOT NULL REFERENCES customer(id) ON DELETE CASCADE,
    purchase_id BIGINT NOT NULL UNIQUE REFERENCES purchase(id) ON DELETE CASCADE,
    amount DECIMAL(20, 8) NOT NULL,
    currency VARCHAR(16) NOT NULL,
    commission_percent SMALLINT NOT NULL CHECK (commission_percent BETWEEN 0 AND 100),
    commission_amount DECIMAL(20, 8) NOT NULL,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_partner_application_status_created_at ON partner_application(status, created_at DESC);
CREATE INDEX idx_partner_referral_partner_id ON partner_referral(partner_id);
CREATE INDEX idx_partner_commission_partner_id ON partner_commission(partner_id);

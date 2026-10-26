CREATE TABLE laveshka_invoice_quote (
    purchase_id BIGINT PRIMARY KEY REFERENCES purchase(id) ON DELETE CASCADE,
    invoice_id TEXT NOT NULL UNIQUE,
    merchant_id BIGINT NOT NULL CHECK (merchant_id > 0),
    rub_minor BIGINT NOT NULL CHECK (rub_minor > 0),
    usd_minor BIGINT NOT NULL CHECK (usd_minor BETWEEN 10 AND 1000000),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

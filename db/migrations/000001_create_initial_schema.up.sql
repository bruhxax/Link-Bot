CREATE TABLE customer
(
    id                BIGSERIAL PRIMARY KEY,
    telegram_id       BIGINT,
    expire_at         TIMESTAMP WITH TIME ZONE,
    created_at        TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    subscription_link TEXT
);

create index idx_customer_telegram_id on customer using hash (telegram_id);

CREATE TABLE purchase
(
    id                 BIGSERIAL PRIMARY KEY,
    amount             DECIMAL(20, 8) NOT NULL,
    customer_id        BIGINT REFERENCES customer (id),
    created_at         TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    month              INTEGER        NOT NULL,
    paid_at            TIMESTAMP WITH TIME ZONE,
    currency           VARCHAR(10),
    expire_at          TIMESTAMP WITH TIME ZONE,
    status             VARCHAR(20),
    invoice_type       VARCHAR(20),
    crypto_invoice_id  BIGINT,
    crypto_invoice_url TEXT,
    yookasa_url        TEXT,
    yookasa_id         uuid
);
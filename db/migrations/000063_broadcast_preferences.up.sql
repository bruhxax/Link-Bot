CREATE TABLE customer_broadcast_unsubscribe (
    customer_id BIGINT PRIMARY KEY REFERENCES customer(id) ON DELETE CASCADE,
    unsubscribed_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

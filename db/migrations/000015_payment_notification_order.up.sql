CREATE TABLE payment_notification_order (
    purchase_id BIGINT PRIMARY KEY REFERENCES purchase(id) ON DELETE CASCADE,
    order_number BIGINT NOT NULL UNIQUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);

INSERT INTO payment_notification_order (purchase_id, order_number)
SELECT id, id
FROM purchase
WHERE status = 'paid'
ON CONFLICT (purchase_id) DO NOTHING;

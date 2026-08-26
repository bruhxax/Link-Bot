CREATE TABLE IF NOT EXISTS p2p_payment_request (
    purchase_id BIGINT PRIMARY KEY REFERENCES purchase(id) ON DELETE CASCADE,
    destination_snapshot JSONB NOT NULL,
    sender_reference TEXT NOT NULL,
    status VARCHAR(16) NOT NULL DEFAULT 'pending',
    notification_chat_id BIGINT,
    notification_message_id BIGINT,
    submitted_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    reviewed_at TIMESTAMPTZ,
    reviewed_by BIGINT,
    CONSTRAINT p2p_payment_request_status_check
        CHECK (status IN ('pending', 'processing', 'approved', 'rejected'))
);

CREATE INDEX IF NOT EXISTS p2p_payment_request_pending_idx
    ON p2p_payment_request (submitted_at, purchase_id)
    WHERE status IN ('pending', 'processing');

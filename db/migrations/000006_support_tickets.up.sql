CREATE TABLE support_ticket
(
    id                    BIGSERIAL PRIMARY KEY,
    customer_id           BIGINT                   NOT NULL REFERENCES customer (id) ON DELETE CASCADE,
    status                VARCHAR(20)              NOT NULL DEFAULT 'open',
    subject               TEXT                     NOT NULL DEFAULT '',
    customer_name         TEXT                     NOT NULL DEFAULT '',
    customer_username     TEXT                     NOT NULL DEFAULT '',
    subscription_label    TEXT                     NOT NULL DEFAULT '',
    created_at            TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at            TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT CURRENT_TIMESTAMP,
    last_message_at       TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT CURRENT_TIMESTAMP,
    closed_at             TIMESTAMP WITH TIME ZONE,
    last_message_preview  TEXT                     NOT NULL DEFAULT '',
    admin_unread_count    INTEGER                  NOT NULL DEFAULT 0,
    customer_unread_count INTEGER                  NOT NULL DEFAULT 0
);

CREATE INDEX idx_support_ticket_customer_status ON support_ticket (customer_id, status, last_message_at DESC);
CREATE INDEX idx_support_ticket_status_last_message ON support_ticket (status, last_message_at DESC);

CREATE TABLE support_message
(
    id                 BIGSERIAL PRIMARY KEY,
    ticket_id          BIGINT                   NOT NULL REFERENCES support_ticket (id) ON DELETE CASCADE,
    author_role        VARCHAR(20)              NOT NULL,
    author_telegram_id BIGINT                   NOT NULL,
    body               TEXT                     NOT NULL,
    created_at         TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_support_message_ticket_created_at ON support_message (ticket_id, created_at ASC);

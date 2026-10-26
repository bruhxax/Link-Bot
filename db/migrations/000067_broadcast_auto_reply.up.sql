CREATE TABLE bot_broadcast_auto_reply (
    id SMALLINT PRIMARY KEY CHECK (id = 1),
    enabled BOOLEAN NOT NULL DEFAULT false,
    text TEXT NOT NULL DEFAULT 'Это автоматическая рассылка. Если у вас есть вопрос, напишите в поддержку.',
    window_hours INTEGER NOT NULL DEFAULT 24 CHECK (window_hours BETWEEN 1 AND 168),
    support_button BOOLEAN NOT NULL DEFAULT true,
    support_button_text TEXT NOT NULL DEFAULT 'Поддержка',
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
INSERT INTO bot_broadcast_auto_reply(id) VALUES(1);

CREATE TABLE broadcast_recipient_reply (
    chat_id BIGINT PRIMARY KEY CHECK (chat_id > 0),
    broadcast_message_id INTEGER NOT NULL CHECK (broadcast_message_id > 0),
    sent_at TIMESTAMPTZ NOT NULL,
    replied_at TIMESTAMPTZ,
    lease_until TIMESTAMPTZ,
    incoming_message_id INTEGER
);

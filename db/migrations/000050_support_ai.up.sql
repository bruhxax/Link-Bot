CREATE TABLE support_ai_state (
    ticket_id BIGINT PRIMARY KEY REFERENCES support_ticket(id) ON DELETE CASCADE,
    processed_message_id BIGINT NOT NULL DEFAULT 0,
    claimed_message_id BIGINT NOT NULL DEFAULT 0,
    lease_until TIMESTAMPTZ,
    pending BOOLEAN NOT NULL DEFAULT TRUE,
    handed_off BOOLEAN NOT NULL DEFAULT FALSE,
    notification_pending BOOLEAN NOT NULL DEFAULT FALSE,
    notification_lease_until TIMESTAMPTZ
);
CREATE INDEX support_ai_pending_idx ON support_ai_state(ticket_id) WHERE pending AND NOT handed_off;
CREATE INDEX support_ai_notification_idx ON support_ai_state(ticket_id) WHERE notification_pending;
CREATE INDEX support_message_ticket_id_idx ON support_message(ticket_id, id DESC);

-- Enqueue in the same transaction as the user's message, including uploads.
CREATE FUNCTION queue_support_ai_message() RETURNS trigger AS $$
BEGIN
    IF NEW.author_role = 'customer' THEN
        INSERT INTO support_ai_state(ticket_id) VALUES (NEW.ticket_id)
        ON CONFLICT (ticket_id) DO UPDATE SET pending = NOT support_ai_state.handed_off;
    ELSIF NEW.author_role = 'admin' THEN
        UPDATE support_ai_state SET handed_off = TRUE, pending = FALSE, notification_pending = FALSE WHERE ticket_id = NEW.ticket_id;
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;
CREATE TRIGGER support_ai_queue AFTER INSERT ON support_message
FOR EACH ROW EXECUTE FUNCTION queue_support_ai_message();

CREATE FUNCTION close_support_ai_ticket() RETURNS trigger AS $$
BEGIN
    IF NEW.status = 'closed' THEN
        UPDATE support_ai_state SET pending = FALSE, lease_until = NULL, notification_pending = FALSE WHERE ticket_id = NEW.id;
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;
CREATE TRIGGER support_ai_close AFTER UPDATE OF status ON support_ticket
FOR EACH ROW EXECUTE FUNCTION close_support_ai_ticket();

DROP TRIGGER IF EXISTS support_ai_queue ON support_message;
DROP FUNCTION IF EXISTS queue_support_ai_message();
DROP TRIGGER IF EXISTS support_ai_close ON support_ticket;
DROP FUNCTION IF EXISTS close_support_ai_ticket();
DROP INDEX IF EXISTS support_message_ticket_id_idx;
-- Older versions only understand customer/admin messages.
UPDATE support_message SET author_role = 'admin' WHERE author_role = 'ai';
DROP TABLE IF EXISTS support_ai_state;

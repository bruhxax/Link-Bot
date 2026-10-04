CREATE INDEX support_ai_message_search_idx ON support_message
USING GIN (to_tsvector('russian', body)) WHERE author_role = 'customer';
CREATE INDEX support_ai_closed_ticket_idx ON support_ticket(id) WHERE status = 'closed';

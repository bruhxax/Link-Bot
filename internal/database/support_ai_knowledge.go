package database

import "context"

type AISupportExample struct {
	Subject, Answer, CustomerName, CustomerUsername string
}

// Search only closed cases containing human replies. Other customers' raw
// questions and account records never leave the repository through this method.
func (r *SupportRepository) FindAISupportExamples(ctx context.Context, customerID, ticketID int64, query string) ([]AISupportExample, error) {
	rows, err := r.pool.Query(ctx, `WITH search AS (SELECT websearch_to_tsquery('russian', $3) AS query),
	 matches AS (
	 SELECT m.ticket_id, MAX(ts_rank(to_tsvector('russian', m.body), search.query)) AS score
	 FROM support_message m CROSS JOIN search JOIN support_ticket t ON t.id = m.ticket_id
	 WHERE m.author_role = 'customer' AND t.status = 'closed' AND t.customer_id <> $1 AND t.id <> $2
	 AND to_tsvector('russian', m.body) @@ search.query GROUP BY m.ticket_id
	 ) SELECT t.subject, reply.body, t.customer_name, t.customer_username
	 FROM matches JOIN support_ticket t ON t.id = matches.ticket_id
	 JOIN LATERAL (SELECT body FROM support_message WHERE ticket_id = t.id AND author_role = 'admin' ORDER BY id DESC LIMIT 1) reply ON TRUE
	 ORDER BY matches.score DESC, t.last_message_at DESC LIMIT 6`, customerID, ticketID, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []AISupportExample{}
	for rows.Next() {
		var item AISupportExample
		if err := rows.Scan(&item.Subject, &item.Answer, &item.CustomerName, &item.CustomerUsername); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

type AICustomerSupportHistory struct {
	TotalTickets int              `json:"totalTickets"`
	OpenTickets  int              `json:"openTickets"`
	Recent       []map[string]any `json:"recent"`
}

func (r *SupportRepository) AICustomerHistory(ctx context.Context, customerID, currentTicketID int64, query string) (AICustomerSupportHistory, error) {
	result := AICustomerSupportHistory{Recent: []map[string]any{}}
	err := r.pool.QueryRow(ctx, `SELECT COUNT(*), COUNT(*) FILTER (WHERE status = 'open') FROM support_ticket WHERE customer_id = $1`, customerID).Scan(&result.TotalTickets, &result.OpenTickets)
	if err != nil {
		return result, err
	}
	// Prefer relevant older conversations, then fill the window with recent ones.
	rows, err := r.pool.Query(ctx, `SELECT t.subject, t.status, t.created_at, t.subscription_label,
	 COALESCE((SELECT jsonb_agg(msg ORDER BY msg.id) FROM (
	  SELECT id, author_role AS role, LEFT(body, 1500) AS text FROM support_message
	  WHERE ticket_id = t.id ORDER BY id DESC LIMIT 8) msg), '[]'::jsonb)
	 FROM support_ticket t WHERE t.customer_id = $1 AND t.id <> $2
	 ORDER BY EXISTS (SELECT 1 FROM support_message m WHERE m.ticket_id = t.id AND m.author_role = 'customer' AND to_tsvector('russian', m.body) @@ websearch_to_tsquery('russian', $3)) DESC,
	 t.last_message_at DESC LIMIT 12`, customerID, currentTicketID, query)
	if err != nil {
		return result, err
	}
	defer rows.Close()
	for rows.Next() {
		var subject, status, subscription string
		var createdAt interface{}
		var messages []map[string]any
		if err := rows.Scan(&subject, &status, &createdAt, &subscription, &messages); err != nil {
			return result, err
		}
		result.Recent = append(result.Recent, map[string]any{"subject": subject, "status": status, "createdAt": createdAt, "subscriptionAtThatTime": subscription, "messages": messages})
	}
	return result, rows.Err()
}

func (r *SupportRepository) AIAccountTotals(ctx context.Context, customerID int64) (map[string]any, error) {
	var total, paid int
	var paidAmount float64
	err := r.pool.QueryRow(ctx, `SELECT COUNT(*), COUNT(*) FILTER (WHERE status = 'paid'), COALESCE(SUM(amount) FILTER (WHERE status = 'paid' AND currency = 'RUB'), 0) FROM purchase WHERE customer_id = $1`, customerID).Scan(&total, &paid, &paidAmount)
	if err != nil {
		return nil, err
	}
	return map[string]any{"totalPurchases": total, "successfulPurchases": paid, "paidRub": paidAmount}, nil
}

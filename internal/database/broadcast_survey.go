package database

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v4"
)

type SurveyQuestion struct {
	Text    string   `json:"text"`
	Options []string `json:"options"`
}
type SurveyConfig struct {
	Enabled    bool             `json:"enabled"`
	Title      string           `json:"title"`
	StartText  string           `json:"startText"`
	Questions  []SurveyQuestion `json:"questions"`
	FinishText string           `json:"finishText"`
}
type SurveyRun struct {
	ID        string            `json:"id"`
	Config    SurveyConfig      `json:"config"`
	Buttons   []BroadcastButton `json:"buttons"`
	Preview   bool              `json:"preview"`
	CreatedAt time.Time         `json:"createdAt"`
	Delivered int               `json:"delivered"`
	Started   int               `json:"started"`
	Completed int               `json:"completed"`
}
type SurveyRecipient struct {
	ID         int64     `json:"id"`
	TelegramID int64     `json:"telegramId"`
	Username   string    `json:"username"`
	Step       int       `json:"step"`
	Answers    []int     `json:"answers"`
	UpdatedAt  time.Time `json:"updatedAt"`
}
type SurveyReport struct {
	Run        SurveyRun         `json:"run"`
	Counts     [][]int           `json:"counts"`
	Recipients []SurveyRecipient `json:"recipients"`
	HasMore    bool              `json:"hasMore"`
}

func (r *BroadcastRepository) SurveyDraft(ctx context.Context) (*SurveyConfig, error) {
	var raw []byte
	if err := r.pool.QueryRow(ctx, "SELECT config FROM broadcast_survey_draft WHERE id=1").Scan(&raw); err != nil {
		return nil, err
	}
	var cfg SurveyConfig
	err := json.Unmarshal(raw, &cfg)
	return &cfg, err
}

func (r *BroadcastRepository) SaveSurvey(ctx context.Context, cfg SurveyConfig, actor int64, expected time.Time) (*BroadcastDraft, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	tag, err := tx.Exec(ctx, `UPDATE bot_broadcast_draft SET updated_by=$1,updated_at=GREATEST(NOW(),updated_at+INTERVAL '1 microsecond')
        WHERE id=1 AND status<>'running' AND updated_at=$2`, actor, expected)
	if err != nil {
		return nil, err
	}
	if tag.RowsAffected() != 1 {
		return nil, nil
	}
	raw, err := json.Marshal(cfg)
	if err != nil {
		return nil, err
	}
	if _, err = tx.Exec(ctx, "UPDATE broadcast_survey_draft SET config=$1 WHERE id=1", string(raw)); err != nil {
		return nil, err
	}
	if err = tx.Commit(ctx); err != nil {
		return nil, err
	}
	draft, err := r.Get(ctx)
	if draft != nil {
		draft.Survey = &cfg
	}
	return draft, err
}

func (r *BroadcastRepository) CreateSurveyRun(ctx context.Context, cfg SurveyConfig, buttons []BroadcastButton, preview bool) (string, error) {
	id := uuid.NewString()
	raw, err := json.Marshal(cfg)
	if err != nil {
		return "", err
	}
	keys, err := json.Marshal(buttons)
	if err != nil {
		return "", err
	}
	_, err = r.pool.Exec(ctx, "INSERT INTO broadcast_survey_run(id,config,buttons,preview) VALUES($1,$2,$3,$4)", id, string(raw), string(keys), preview)
	return id, err
}
func (r *BroadcastRepository) RegisterSurveyRecipient(ctx context.Context, run string, chat int64, message int) error {
	_, err := r.pool.Exec(ctx, "INSERT INTO broadcast_survey_session(run_id,telegram_id,message_id) VALUES($1,$2,$3)", run, chat, message)
	return err
}
func (r *BroadcastRepository) SurveyRun(ctx context.Context, id string) (*SurveyRun, error) {
	var run SurveyRun
	var raw, buttons []byte
	err := r.pool.QueryRow(ctx, "SELECT id::text,config,buttons,preview,created_at FROM broadcast_survey_run WHERE id=$1", id).Scan(&run.ID, &raw, &buttons, &run.Preview, &run.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if err = json.Unmarshal(raw, &run.Config); err != nil {
		return nil, err
	}
	if err = json.Unmarshal(buttons, &run.Buttons); err != nil {
		return nil, err
	}
	return &run, nil
}

// Lock each recipient while advancing. Stale double clicks return the saved
// stage, so they can repair a failed Telegram edit without recording twice.
func (r *BroadcastRepository) AdvanceSurvey(ctx context.Context, run string, chat int64, message, expected, option, total int, optionCount int, render ...func(int) error) (int, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback(ctx)
	var step int
	var raw []byte
	err = tx.QueryRow(ctx, "SELECT step,answers FROM broadcast_survey_session WHERE run_id=$1 AND telegram_id=$2 AND message_id=$3 FOR UPDATE", run, chat, message).Scan(&step, &raw)
	if err != nil {
		return 0, err
	}
	if expected != step || step >= total {
		if len(render) > 0 {
			if err = render[0](step); err != nil {
				return 0, err
			}
		}
		return step, tx.Commit(ctx)
	}
	var answers []int
	if err = json.Unmarshal(raw, &answers); err != nil {
		return 0, err
	}
	if step >= 0 {
		if option < 0 || option >= optionCount {
			return 0, errors.New("invalid survey answer")
		}
		answers = append(answers, option)
	} else if option != -1 {
		return 0, errors.New("invalid survey start")
	}
	encoded, _ := json.Marshal(answers)
	_, err = tx.Exec(ctx, "UPDATE broadcast_survey_session SET step=step+1,answers=$4,updated_at=NOW() WHERE run_id=$1 AND telegram_id=$2 AND message_id=$3", run, chat, message, string(encoded))
	if err != nil {
		return 0, err
	}
	if len(render) > 0 {
		if err = render[0](step + 1); err != nil {
			return 0, err
		}
	}
	return step + 1, tx.Commit(ctx)
}

func (r *BroadcastRepository) SurveyRuns(ctx context.Context) ([]SurveyRun, error) {
	rows, err := r.pool.Query(ctx, `SELECT r.id::text,r.config,r.created_at,count(s.id),count(s.id) FILTER(WHERE s.step>=0),
        count(s.id) FILTER(WHERE s.step>=jsonb_array_length(r.config->'questions'))
        FROM broadcast_survey_run r LEFT JOIN broadcast_survey_session s ON s.run_id=r.id
        WHERE NOT r.preview GROUP BY r.id ORDER BY r.created_at DESC LIMIT 30`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	runs := []SurveyRun{}
	for rows.Next() {
		var run SurveyRun
		var raw []byte
		if err = rows.Scan(&run.ID, &raw, &run.CreatedAt, &run.Delivered, &run.Started, &run.Completed); err != nil {
			return nil, err
		}
		if err = json.Unmarshal(raw, &run.Config); err != nil {
			return nil, err
		}
		runs = append(runs, run)
	}
	return runs, rows.Err()
}
func (r *BroadcastRepository) SurveyReport(ctx context.Context, id string, before int64) (*SurveyReport, error) {
	run, err := r.SurveyRun(ctx, id)
	if err != nil || run == nil || run.Preview {
		return nil, err
	}
	report := &SurveyReport{Run: *run, Counts: make([][]int, len(run.Config.Questions)), Recipients: []SurveyRecipient{}}
	for i, q := range run.Config.Questions {
		report.Counts[i] = make([]int, len(q.Options))
	}
	err = r.pool.QueryRow(ctx, `SELECT count(*),count(*) FILTER(WHERE step>=0),count(*) FILTER(WHERE step>=$2)
        FROM broadcast_survey_session WHERE run_id=$1`, id, len(run.Config.Questions)).Scan(&report.Run.Delivered, &report.Run.Started, &report.Run.Completed)
	if err != nil {
		return nil, err
	}
	rows, err := r.pool.Query(ctx, `SELECT (a.ordinality-1)::int,(a.value::text)::int,count(*) FROM broadcast_survey_session s
        CROSS JOIN LATERAL jsonb_array_elements(s.answers) WITH ORDINALITY a(value,ordinality)
        WHERE s.run_id=$1 GROUP BY 1,2`, id)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var q, o, n int
		if err = rows.Scan(&q, &o, &n); err != nil {
			rows.Close()
			return nil, err
		}
		if q >= 0 && q < len(report.Counts) && o >= 0 && o < len(report.Counts[q]) {
			report.Counts[q][o] = n
		}
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, err
	}
	rows, err = r.pool.Query(ctx, `SELECT s.id,s.telegram_id,COALESCE(c.username,''),s.step,s.answers,s.updated_at
        FROM broadcast_survey_session s LEFT JOIN customer c ON c.telegram_id=s.telegram_id
        WHERE s.run_id=$1 AND ($2::bigint=0 OR s.id<$2) ORDER BY s.id DESC LIMIT 51`, id, before)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var p SurveyRecipient
		var raw []byte
		if err = rows.Scan(&p.ID, &p.TelegramID, &p.Username, &p.Step, &raw, &p.UpdatedAt); err != nil {
			return nil, err
		}
		if err = json.Unmarshal(raw, &p.Answers); err != nil {
			return nil, err
		}
		report.Recipients = append(report.Recipients, p)
	}
	if len(report.Recipients) > 50 {
		report.HasMore = true
		report.Recipients = report.Recipients[:50]
	}
	return report, rows.Err()
}

CREATE TABLE broadcast_survey_draft (
    id INTEGER PRIMARY KEY CHECK (id=1),
    config JSONB NOT NULL DEFAULT '{}'
);
INSERT INTO broadcast_survey_draft(id) VALUES(1);
CREATE TABLE broadcast_survey_run (
    id UUID PRIMARY KEY,
    config JSONB NOT NULL,
    buttons JSONB NOT NULL DEFAULT '[]',
    preview BOOLEAN NOT NULL DEFAULT FALSE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE TABLE broadcast_survey_session (
    id BIGSERIAL UNIQUE NOT NULL,
    run_id UUID NOT NULL REFERENCES broadcast_survey_run(id) ON DELETE CASCADE,
    telegram_id BIGINT NOT NULL,
    message_id INTEGER NOT NULL,
    step INTEGER NOT NULL DEFAULT -1 CHECK (step>=-1),
    answers JSONB NOT NULL DEFAULT '[]',
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY(run_id,telegram_id)
);
CREATE INDEX broadcast_survey_session_page ON broadcast_survey_session(run_id,id DESC);

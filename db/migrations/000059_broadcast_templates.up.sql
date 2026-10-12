CREATE TABLE broadcast_template (
 id BIGSERIAL PRIMARY KEY,
 channel TEXT NOT NULL CHECK (channel IN ('telegram','email')),
 name TEXT NOT NULL CHECK (length(name) BETWEEN 1 AND 80),
 payload JSONB NOT NULL,
 created_by BIGINT NOT NULL,
 created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE customer_email_auth (
  customer_id BIGINT PRIMARY KEY REFERENCES customer(id) ON DELETE CASCADE,
  email TEXT NOT NULL,
  password_hash TEXT NOT NULL,
  verified_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  CONSTRAINT customer_email_auth_email_unique UNIQUE (email)
);

CREATE TABLE email_auth_challenge (
  id UUID PRIMARY KEY,
  email TEXT NOT NULL UNIQUE,
  mode TEXT NOT NULL CHECK (mode IN ('login', 'register')),
  password_hash TEXT,
  code_hash TEXT NOT NULL,
  attempts SMALLINT NOT NULL DEFAULT 0,
  expires_at TIMESTAMPTZ NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

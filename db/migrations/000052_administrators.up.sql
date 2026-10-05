CREATE TABLE administrator (
    customer_id BIGINT PRIMARY KEY REFERENCES customer(id) ON DELETE CASCADE,
    role_name TEXT NOT NULL CHECK (char_length(role_name) BETWEEN 1 AND 60),
    color TEXT NOT NULL CHECK (color ~ '^#[0-9a-fA-F]{6}$'),
    permissions TEXT[] NOT NULL DEFAULT '{}',
    updated_by BIGINT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TRIGGER miniapp_changed_administrators
AFTER INSERT OR UPDATE OR DELETE ON administrator
FOR EACH STATEMENT EXECUTE FUNCTION miniapp_notify_change();

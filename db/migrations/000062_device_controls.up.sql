ALTER TABLE device_traffic_binding ADD COLUMN personal BOOLEAN NOT NULL DEFAULT FALSE;
ALTER TABLE device_traffic_binding ADD COLUMN blocked JSONB NOT NULL DEFAULT '[]'::jsonb;
-- Preserve the last valid profile while paused; only the subscription bearer
-- and matching HWID/client format can retrieve this private configuration.
CREATE TABLE device_profile_config (
 binding_id BIGINT NOT NULL REFERENCES device_traffic_binding(id) ON DELETE CASCADE,
 slot_number INT NOT NULL,
 format_key TEXT NOT NULL,
 hwid TEXT NOT NULL,
 source_hash TEXT NOT NULL,
 body BYTEA NOT NULL,
 headers JSONB NOT NULL,
 updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
 PRIMARY KEY(binding_id,slot_number,format_key)
);

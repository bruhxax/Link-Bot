CREATE TABLE node_outage (
    node_uuid TEXT PRIMARY KEY,
    node_name TEXT NOT NULL,
    offline_since TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    down_notified BOOLEAN NOT NULL DEFAULT FALSE
);

CREATE TABLE node_hidden_host (
    node_uuid TEXT NOT NULL REFERENCES node_outage(node_uuid) ON DELETE CASCADE,
    host_uuid TEXT NOT NULL,
    host_remark TEXT NOT NULL DEFAULT '',
    PRIMARY KEY (node_uuid, host_uuid)
);

CREATE TABLE subscription_reissue (
    id BIGSERIAL PRIMARY KEY,
    subscription_id BIGINT NOT NULL,
    telegram_id BIGINT NOT NULL,
    old_panel_user_id BIGINT,
    old_panel_user_uuid UUID,
    new_link TEXT NOT NULL,
    delete_after TIMESTAMPTZ NOT NULL,
    notified_at TIMESTAMPTZ,
    last_notification_attempt_at TIMESTAMPTZ,
    deleted_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CHECK (old_panel_user_id IS NOT NULL OR old_panel_user_uuid IS NOT NULL)
);
CREATE INDEX subscription_reissue_pending_notification_idx ON subscription_reissue (id) WHERE notified_at IS NULL;
CREATE INDEX subscription_reissue_pending_delete_idx ON subscription_reissue (delete_after) WHERE deleted_at IS NULL;

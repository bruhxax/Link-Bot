-- Notify the Mini App after committed changes, regardless of whether they came
-- from its HTTP API, the Telegram bot, a payment webhook, or a background job.
CREATE OR REPLACE FUNCTION miniapp_notify_change() RETURNS trigger AS $$
BEGIN
    PERFORM pg_notify('miniapp_changes', TG_TABLE_NAME);
    RETURN NULL;
END;
$$ LANGUAGE plpgsql;

DO $$
DECLARE
    item record;
BEGIN
    FOR item IN
        SELECT tablename FROM pg_tables
        WHERE schemaname = current_schema()
          AND tablename NOT IN ('schema_migrations', 'app_release_notification_state')
    LOOP
        EXECUTE format('CREATE TRIGGER miniapp_changed_insert_delete AFTER INSERT OR DELETE ON %I FOR EACH ROW EXECUTE FUNCTION miniapp_notify_change()', item.tablename);
        EXECUTE format('CREATE TRIGGER miniapp_changed_update AFTER UPDATE ON %I FOR EACH ROW WHEN (OLD.* IS DISTINCT FROM NEW.*) EXECUTE FUNCTION miniapp_notify_change()', item.tablename);
    END LOOP;
END;
$$;

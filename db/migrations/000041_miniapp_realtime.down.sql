DO $$
DECLARE
    item record;
BEGIN
    FOR item IN SELECT tablename FROM pg_tables WHERE schemaname = current_schema() LOOP
        EXECUTE format('DROP TRIGGER IF EXISTS miniapp_changed_insert_delete ON %I', item.tablename);
        EXECUTE format('DROP TRIGGER IF EXISTS miniapp_changed_update ON %I', item.tablename);
    END LOOP;
END;
$$;

DROP FUNCTION IF EXISTS miniapp_notify_change();

CREATE TABLE user_activity (
    id BIGSERIAL PRIMARY KEY,
    customer_id BIGINT NOT NULL REFERENCES customer(id) ON DELETE CASCADE,
    source TEXT NOT NULL CHECK (source IN ('miniapp', 'telegram', 'payment')),
    action TEXT NOT NULL,
    title TEXT NOT NULL,
    status TEXT NOT NULL CHECK (status IN ('info', 'success', 'failed', 'pending')),
    details JSONB NOT NULL DEFAULT '[]',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX user_activity_customer_id_idx ON user_activity(customer_id, id DESC);

-- Record committed payment transitions from every provider, including bot and
-- automatic payments. A repeated webhook that leaves the status unchanged is
-- not another event. Do not copy checkout URLs, tokens or payment credentials.
CREATE FUNCTION record_purchase_activity() RETURNS TRIGGER AS $$
DECLARE
    event_title TEXT;
    event_status TEXT;
BEGIN
    IF TG_OP = 'UPDATE' AND OLD.status IS NOT DISTINCT FROM NEW.status THEN
        RETURN NEW;
    END IF;
    event_title := CASE NEW.status
        WHEN 'paid' THEN 'Платёж оплачен'
        WHEN 'cancel' THEN 'Платёж отменён'
        WHEN 'pending' THEN 'Платёж ожидает оплаты'
        ELSE 'Создан платёж'
    END;
    event_status := CASE NEW.status
        WHEN 'paid' THEN 'success'
        WHEN 'cancel' THEN 'info'
        ELSE 'pending'
    END;
    INSERT INTO user_activity(customer_id,source,action,title,status,details)
    VALUES(NEW.customer_id,'payment','payment.' || NEW.status,event_title,event_status,
        jsonb_build_array(
            jsonb_build_object('label','Платёж','value','#' || NEW.id),
            jsonb_build_object('label','Сумма','value',NEW.amount || ' ' || NEW.currency),
            jsonb_build_object('label','Способ','value',NEW.invoice_type),
            jsonb_build_object('label','Тариф','value',COALESCE(NEW.plan_id,'')),
            jsonb_build_object('label','Тип покупки','value',COALESCE(NEW.purchase_kind,'subscription')),
            jsonb_build_object('label','Автоплатёж','value',CASE WHEN NEW.is_auto_payment THEN 'Да' ELSE 'Нет' END)
        ));
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;
CREATE TRIGGER user_activity_purchase AFTER INSERT OR UPDATE OF status ON purchase
    FOR EACH ROW EXECUTE FUNCTION record_purchase_activity();

CREATE TRIGGER miniapp_user_activity_changed AFTER INSERT ON user_activity
    FOR EACH ROW EXECUTE FUNCTION miniapp_notify_change();

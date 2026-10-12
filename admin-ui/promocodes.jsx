import React, { useState, useRef, useLayoutEffect } from "react";
import {
  ActionIcon,
  Alert,
  Badge,
  Button,
  Group,
  Modal,
  NumberInput,
  Stack,
  Switch,
  Text,
  TextInput,
  Tooltip,
} from "@mantine/core";
import {
  PiPercent,
  PiCoins,
  PiCrown,
  PiTag,
  PiUsers,
  PiCalendarBlank,
  PiPlusCircle,
  PiMagnifyingGlass,
  PiPencilSimple,
  PiTrash,
  PiInfo,
} from "react-icons/pi";
import {
  initialPromoDraft,
  promoCreatePayload,
  promoSettingsPayload,
  promoStatus,
  promoReward,
  promoRows,
  promoLocalDate,
} from "./promocodes-data.mjs";
import "./promocodes.css";
const statuses = {
  active: ["Активен", "teal"],
  expired: ["Истёк", "red"],
  inactive: ["Отключён", "gray"],
  exhausted: ["Исчерпан", "orange"],
};
const date = (v, full = false) =>
  v
    ? new Date(v).toLocaleString(
        "ru",
        full
          ? { day: "2-digit", month: "2-digit", year: "2-digit" }
          : {
              day: "2-digit",
              month: "short",
              hour: "2-digit",
              minute: "2-digit",
            },
      )
    : "Без срока";
function Private({ hidden, children }) {
  return (
    <span className="rn-private-value" aria-hidden={hidden}>
      {children}
    </span>
  );
}
function Label({ text, hint }) {
  return (
    <Group gap={7} component="span">
      {text}
      <Tooltip label={hint} multiline w={230} withArrow>
        <span className="rn-promo-help" tabIndex={0} aria-label={hint}>
          <PiInfo size={15} />
        </span>
      </Tooltip>
    </Group>
  );
}
function SettingsFields({ draft, set, disabled, personal = false }) {
  return (
    <>
      <NumberInput
        label={
          <Label
            text="Лимит пользователей"
            hint="Сколько разных пользователей могут применить код. Пустое поле или 0 — без лимита."
          />
        }
        aria-label="Лимит пользователей"
        placeholder="Необязательно"
        min={0}
        max={2147483647}
        allowDecimal={false}
        allowNegative={false}
        hideControls
        leftSection={<PiUsers size={20} />}
        value={draft.limit}
        onChange={(v) => set("limit", v)}
        disabled={disabled || personal}
      />
      <TextInput
        label={
          <Label
            text="Действует до"
            hint="Время по часовому поясу вашего браузера. Пустое поле — без срока."
          />
        }
        aria-label="Действует до"
        type="datetime-local"
        leftSection={<PiCalendarBlank size={20} />}
        value={draft.expires}
        onChange={(e) => set("expires", e.target.value)}
        disabled={disabled}
      />
    </>
  );
}
function Edit({ item, model, onClose }) {
  const [draft, setDraft] = useState({
      limit: item.maxRedemptions || "",
      expires: promoLocalDate(item.expiresAt),
      active: item.isActive ?? item.status !== "inactive",
    }),
    [error, setError] = useState(""),
    [busy, setBusy] = useState(false);
  const set = (k, v) => setDraft((d) => ({ ...d, [k]: v }));
  const save = async (e) => {
    e.preventDefault();
    if (busy) return;
    setError("");
    try {
      const payload = {
        id: item.id,
        updatedAt: item.updatedAt,
        isActive: draft.active,
        ...promoSettingsPayload(draft),
      };
      setBusy(true);
      await model.update(payload);
      onClose();
    } catch (e) {
      setError(e.message);
    } finally {
      setBusy(false);
    }
  };
  return (
    <Modal
      opened
      onClose={() => !busy && onClose()}
      title={`Настройки «${item.code}»`}
      closeButtonProps={{ "aria-label": "Закрыть настройки промокода" }}
      size="lg"
      className="rn-promo-modal"
    >
      <form onSubmit={save}>
        <Stack>
          <Text size="sm" c="dimmed">
            {promoReward(item)} · {item.redemptionCount || 0} использований. Код
            и бонус сохраняются.
          </Text>
          <div className="rn-promo-fields">
            <SettingsFields
              draft={draft}
              set={set}
              disabled={busy}
              personal={item.isPersonal}
            />
          </div>
          {item.isPersonal && (
            <Text size="sm" c="dimmed">
              Персональный код доступен одному получателю и имеет одно
              использование.
            </Text>
          )}
          <Switch
            label="Промокод включён"
            color="teal"
            checked={draft.active}
            onChange={(e) => set("active", e.currentTarget.checked)}
            disabled={busy}
          />
          {error && <Alert color="red">{error}</Alert>}
          <Group justify="flex-end">
            <Button variant="default" onClick={onClose} disabled={busy}>
              Отмена
            </Button>
            <Button type="submit" loading={busy} disabled={!item.updatedAt}>
              Сохранить
            </Button>
          </Group>
        </Stack>
      </form>
    </Modal>
  );
}
export function Promocodes({ model: root, hidden }) {
  const model = root.promoConsole || {},
    [draft, setDraft] = useState(() => ({
      ...initialPromoDraft,
      ...model.draft,
    })),
    [query, setQuery] = useState(""),
    [error, setError] = useState(""),
    [busy, setBusy] = useState(false),
    [editing, setEditing] = useState(null),
    [deleting, setDeleting] = useState(null),
    [deleteBusy, setDeleteBusy] = useState(false),
    [deleteError, setDeleteError] = useState("");
  const stage = useRef(null);
  const type = draft.type === "subscription" ? draft.unit : draft.type;
  useLayoutEffect(() => {
    if (
      !stage.current ||
      matchMedia("(prefers-reduced-motion: reduce)").matches
    )
      return;
    const a = stage.current.animate(
      [
        { opacity: 0, transform: "translateY(5px)" },
        { opacity: 1, transform: "translateY(0)" },
      ],
      { duration: 230, easing: "cubic-bezier(.22,1,.36,1)" },
    );
    return () => a.cancel();
  }, [type]);
  const set = (key, value) =>
    setDraft((d) => {
      const next = { ...d, [key]: value };
      model.storeDraft?.(next);
      return next;
    });
  const create = async (e) => {
    e.preventDefault();
    if (busy) return;
    setError("");
    try {
      const payload = promoCreatePayload(draft);
      setBusy(true);
      await model.create(payload);
      const next = { ...draft, code: "", limit: "", expires: "" };
      setDraft(next);
      model.storeDraft?.(next);
    } catch (e) {
      setError(e.message);
    } finally {
      setBusy(false);
    }
  };
  const remove = async () => {
    if (deleteBusy) return;
    setDeleteError("");
    setDeleteBusy(true);
    try {
      await model.remove(deleting.id);
      setDeleting(null);
    } catch (e) {
      setDeleteError(e.message);
    } finally {
      setDeleteBusy(false);
    }
  };
  const valueKey =
      type === "discount"
        ? "discount"
        : type === "balance"
          ? "balance"
          : type === "traffic"
            ? "traffic"
            : "days",
    valueLabel =
      type === "discount"
        ? "Скидка, %"
        : type === "balance"
          ? "На баланс, ₽"
          : type === "traffic"
            ? "ГБ к подписке"
            : "Дней к подписке";
  const items = model.items || [],
    rows = promoRows(items, query);
  const disabled = busy || !model.canManage;
  return (
    <div className="rn-promocodes-section">
      <section className="rn-promo-card rn-promo-create">
        <form onSubmit={create}>
          <header>
            <div>
              <h2>Создать промокод</h2>
              <p>
                Выберите тип промокода и укажите параметры. Их можно изменить до
                создания.
              </p>
            </div>
            <div
              className="rn-promo-type-tabs"
              role="tablist"
              aria-label="Тип промокода"
            >
              {[
                ["discount", "Скидка", PiPercent],
                ["balance", "Баланс", PiCoins],
                ["subscription", "Подписка", PiCrown],
              ].map(([id, label, Icon]) => (
                <button
                  key={id}
                  type="button"
                  role="tab"
                  aria-selected={draft.type === id}
                  className={draft.type === id ? "is-active" : ""}
                  onClick={() => set("type", id)}
                  disabled={disabled}
                >
                  <Icon size={23} />
                  {label}
                </button>
              ))}
            </div>
          </header>
          {draft.type === "subscription" && (
            <div
              className="rn-promo-unit-tabs"
              role="tablist"
              aria-label="Бонус к подписке"
            >
              {[
                ["days", "Дни"],
                ["traffic", "Трафик, ГБ"],
                ["days_traffic", "Дни + ГБ"],
              ].map(([id, label]) => (
                <button
                  key={id}
                  type="button"
                  role="tab"
                  aria-selected={draft.unit === id}
                  className={draft.unit === id ? "is-active" : ""}
                  onClick={() => set("unit", id)}
                  disabled={disabled}
                >
                  {label}
                </button>
              ))}
            </div>
          )}
          <div className="rn-promo-fields" ref={stage}>
            <TextInput
              label={
                <Label
                  text="Код"
                  hint="От 3 до 32 латинских букв, цифр, дефисов или подчёркиваний. Регистр не важен."
                />
              }
              aria-label="Код"
              maxLength={32}
              placeholder="Введите промокод"
              leftSection={<PiTag size={20} />}
              value={draft.code}
              onChange={(e) => set("code", e.target.value.toUpperCase())}
              disabled={disabled}
            />
            <NumberInput
              label={
                <Label
                  text={valueLabel}
                  hint={
                    type === "discount"
                      ? "Скидка от 1 до 99% на покупку."
                      : "Бонус за однократное применение промокода."
                  }
                />
              }
              aria-label={valueLabel}
              min={1}
              max={
                type === "discount"
                  ? 99
                  : ["days", "days_traffic"].includes(type)
                    ? 3650
                    : 1000000
              }
              allowNegative={false}
              allowDecimal={false}
              hideControls
              rightSection={
                <span>
                  {type === "discount"
                    ? "%"
                    : type === "balance"
                      ? "₽"
                      : type === "traffic"
                        ? "ГБ"
                        : "дн."}
                </span>
              }
              value={draft[valueKey]}
              onChange={(v) => set(valueKey, v)}
              disabled={disabled}
            />
            {type === "days_traffic" && (
              <NumberInput
                label="ГБ к подписке"
                min={1}
                max={1000000}
                allowDecimal={false}
                allowNegative={false}
                value={draft.combinedTraffic}
                onChange={(v) => set("combinedTraffic", v)}
                disabled={disabled}
              />
            )}
            <SettingsFields draft={draft} set={set} disabled={disabled} />
          </div>
          {error && (
            <Alert color="red" className="rn-promo-form-error">
              {error}
            </Alert>
          )}
          <footer>
            <Button
              type="submit"
              size="md"
              loading={busy}
              disabled={disabled}
              leftSection={<PiPlusCircle size={21} />}
            >
              Создать промокод
            </Button>
          </footer>
        </form>
      </section>
      <section className="rn-promo-card rn-promo-list">
        <header>
          <Group gap={12}>
            <h2>Активные и созданные коды</h2>
            <Badge variant="light" color="gray">
              <Private hidden={hidden}>{items.length}</Private>
            </Badge>
          </Group>
          <TextInput
            aria-label="Поиск по коду"
            placeholder="Поиск по коду…"
            leftSection={<PiMagnifyingGlass size={20} />}
            value={query}
            onChange={(e) => setQuery(e.target.value)}
          />
        </header>
        <div className="rn-promo-rows">
          {rows.map((item) => {
            const status = promoStatus(item),
              [label, color] = statuses[status];
            return (
              <article className={`rn-promo-row is-${status}`} key={item.id}>
                <div className="rn-promo-row-main">
                  <Group gap={18}>
                    <strong>{item.code}</strong>
                    <Badge color={color} variant="light">
                      <i />
                      {label}
                    </Badge>
                  </Group>
                  <div className="rn-promo-row-meta">
                    <b>
                      <Private hidden={hidden}>{promoReward(item)}</Private>
                    </b>
                    <span>
                      <Private hidden={hidden}>
                        {Number(item.redemptionCount || 0).toLocaleString("ru")}{" "}
                        использ.
                      </Private>
                    </span>
                    <span>
                      <Private hidden={hidden}>
                        {item.maxRedemptions > 0
                          ? `Лимит ${Number(item.maxRedemptions).toLocaleString("ru")}`
                          : "Без лимита"}
                      </Private>
                    </span>
                    <span>
                      {item.expiresAt
                        ? `Действует до ${date(item.expiresAt, true)}`
                        : "Без срока"}
                    </span>
                  </div>
                </div>
                <div className="rn-promo-row-actions">
                  <time dateTime={item.createdAt}>{date(item.createdAt)}</time>
                  <Tooltip label="Настройки промокода">
                    <ActionIcon
                      aria-label={`Редактировать ${item.code}`}
                      variant="default"
                      size={42}
                      disabled={!model.canManage}
                      onClick={() => setEditing(item)}
                    >
                      <PiPencilSimple size={21} />
                    </ActionIcon>
                  </Tooltip>
                  <Tooltip label="Удалить промокод">
                    <ActionIcon
                      aria-label={`Удалить ${item.code}`}
                      color="red"
                      variant="light"
                      size={42}
                      disabled={!model.canManage}
                      onClick={() => {
                        setDeleting(item);
                        setDeleteError("");
                      }}
                    >
                      <PiTrash size={21} />
                    </ActionIcon>
                  </Tooltip>
                </div>
              </article>
            );
          })}
        </div>
        {!rows.length && (
          <div className="rn-promo-empty">
            {items.length
              ? "По этому коду ничего не найдено."
              : "Промокодов пока нет. Создайте первый код выше."}
          </div>
        )}
      </section>
      {editing && (
        <Edit
          key={editing.id}
          item={editing}
          model={model}
          onClose={() => setEditing(null)}
        />
      )}
      <Modal
        opened={Boolean(deleting)}
        onClose={() => !deleteBusy && setDeleting(null)}
        title={`Удалить «${deleting?.code || ""}»?`}
        closeButtonProps={{ "aria-label": "Закрыть удаление промокода" }}
        size="md"
        className="rn-promo-modal"
      >
        <Stack>
          <Text size="sm">
            Код больше нельзя будет применить. Ранее выданные бонусы и история
            использований сохранятся.
          </Text>
          {deleteError && <Alert color="red">{deleteError}</Alert>}
          <Group justify="flex-end">
            <Button
              variant="default"
              disabled={deleteBusy}
              onClick={() => setDeleting(null)}
            >
              Отмена
            </Button>
            <Button color="red" loading={deleteBusy} onClick={remove}>
              Удалить
            </Button>
          </Group>
        </Stack>
      </Modal>
    </div>
  );
}

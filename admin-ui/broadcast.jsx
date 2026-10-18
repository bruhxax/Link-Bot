import React, {
  useMemo,
  useState,
  useLayoutEffect,
  useRef,
  useId,
} from "react";
import {
  Accordion,
  ActionIcon,
  Alert,
  Badge,
  Button,
  Checkbox,
  Group,
  Modal,
  MultiSelect,
  Radio,
  Select,
  Stack,
  Switch,
  Text,
  TextInput,
  Textarea,
  Tooltip,
} from "@mantine/core";
import {
  PiTelegramLogo,
  PiUsers,
  PiWarningCircle,
  PiEnvelopeSimple,
  PiBookmarkSimple,
  PiPlus,
  PiTrash,
  PiGear,
  PiEye,
  PiArrowsClockwise,
  PiCheck,
  PiFileText,
  PiImage,
  PiVideo,
  PiMusicNote,
} from "react-icons/pi";
import { animateAdminEntrance } from "./admin-entrance.mjs";
import {
  BROADCAST_BUTTON_TYPES,
  BROADCAST_CATEGORIES,
  BROADCAST_STYLES,
  BROADCAST_STATUS,
  broadcastMetrics,
  broadcastButtonText,
} from "./broadcast-data.mjs";
import "./broadcast.css";

const number = (value) => Number(value || 0).toLocaleString("ru");
function Private({ hidden, children }) {
  return (
    <span className="rn-private-value" aria-hidden={hidden}>
      {children}
    </span>
  );
}
function Stats({ draft, loaded, hidden }) {
  const m = broadcastMetrics(draft);
  return (
    <div className="rn-broadcast-metrics">
      {[
        [
          "Всего получателей",
          m.total,
          PiTelegramLogo,
          "#409dff",
          "По последней рассылке",
        ],
        ["Доставлено", m.sent, PiUsers, "#02d2b3", "Успешно отправлено"],
        [
          "Ошибок",
          m.failed,
          PiWarningCircle,
          "#fb596f",
          `${m.errorPercent}% от получателей`,
        ],
      ].map(([title, value, Icon, color, note]) => (
        <div
          className="rn-broadcast-metric"
          key={title}
          style={{ "--broadcast-color": color }}
        >
          <div>
            <Icon size={31} />
          </div>
          <section>
            <span>{title}</span>
            <strong>
              <Private hidden={hidden}>{loaded ? number(value) : "—"}</Private>
            </strong>
            <small>
              {title === "Ошибок" ? (
                <Private hidden={hidden}>{loaded ? note : "—"}</Private>
              ) : (
                note
              )}
            </small>
          </section>
        </div>
      ))}
    </div>
  );
}
function Audience({ model, disabled, hidden }) {
  const a = model.audience || {},
    [mode, setMode] = useState(
      a.tag && a.category === "all" ? "tag" : a.category || "all",
    );
  const choose = (value) => {
    setMode(value);
    if (value !== "tag") model.updateAudience("tag", "");
    model.updateAudience("category", value === "tag" ? "all" : value);
  };
  useLayoutEffect(
    () =>
      setMode(
        model.audience?.tag && model.audience?.category === "all"
          ? "tag"
          : model.audience?.category || "all",
      ),
    [model.audience],
  );
  const squad = (items) =>
    (items || []).map((s) => ({ value: s.uuid, label: s.name }));
  return (
    <div className="rn-broadcast-audience">
      <Text size="sm" mb={8}>
        Кому отправлять
      </Text>
      <Radio.Group value={mode} onChange={choose}>
        <Group gap="lg" wrap="wrap">
          {[
            ...BROADCAST_CATEGORIES.slice(0, 4),
            { value: "tag", label: "По тегам" },
          ].map((c) => (
            <Radio
              key={c.value}
              value={c.value}
              label={c.label}
              color="teal"
              disabled={disabled}
            />
          ))}
        </Group>
      </Radio.Group>
      {mode === "tag" && (
        <TextInput
          label="Тег в панели"
          placeholder="Например, PREMIUM"
          mt="sm"
          maxLength={64}
          value={a.tag || ""}
          onChange={(e) => model.updateAudience("tag", e.target.value)}
          disabled={disabled}
        />
      )}
      <Accordion variant="separated" className="rn-broadcast-advanced" mt="md">
        <Accordion.Item value="audience">
          <Accordion.Control icon={<PiGear size={18} />}>
            Дополнительный отбор получателей
          </Accordion.Control>
          <Accordion.Panel>
            <div className="rn-broadcast-two-fields">
              <Select
                label="Категория"
                data={BROADCAST_CATEGORIES}
                value={a.category || "all"}
                onChange={choose}
                disabled={disabled}
              />
              <Select
                label="Язык"
                data={[
                  { value: "", label: "Любой" },
                  { value: "ru", label: "Русский" },
                  { value: "en", label: "English" },
                  { value: "fa", label: "فارسی" },
                ]}
                value={a.language || ""}
                onChange={(value) =>
                  model.updateAudience("language", value || "")
                }
                disabled={disabled}
              />
              <TextInput
                label="Тег в панели"
                value={a.tag || ""}
                placeholder="Любой тег"
                onChange={(e) => model.updateAudience("tag", e.target.value)}
                disabled={disabled}
              />
            </div>
            <div className="rn-broadcast-two-fields">
              <MultiSelect
                label="Внутренние сквады"
                searchable
                data={squad(model.audienceOptions?.internal)}
                value={a.internalSquads || []}
                onChange={(value) =>
                  model.updateAudience("internalSquads", value)
                }
                disabled={disabled}
              />
              <MultiSelect
                label="Внешние сквады"
                searchable
                data={squad(model.audienceOptions?.external)}
                value={a.externalSquads || []}
                onChange={(value) =>
                  model.updateAudience("externalSquads", value)
                }
                disabled={disabled}
              />
            </div>
            <Button
              variant="subtle"
              size="xs"
              onClick={() => model.loadAudience()}
              disabled={disabled}
            >
              Обновить сквады
            </Button>
          </Accordion.Panel>
        </Accordion.Item>
      </Accordion>
      {model.channel === "telegram" && (
        <Switch
          mt="md"
          label="Игнорировать отписку от рассылки"
          description="Отправить эту Telegram-рассылку также отписавшимся пользователям. Их настройки останутся прежними."
          checked={Boolean(a.ignoreUnsubscribe)}
          onChange={(event) => model.updateAudience("ignoreUnsubscribe", event.currentTarget.checked)}
          disabled={disabled}
          color="teal"
        />
      )}
      <div className="rn-broadcast-audience-count">
        <span>
          Получателей:{" "}
          <Private hidden={hidden}>
            {model.audienceCount == null
              ? "Не подсчитано"
              : number(model.audienceCount)}
          </Private>
        </span>
        <Button
          variant="subtle"
          size="xs"
          onClick={() => model.checkAudience()}
          disabled={disabled}
          loading={model.audienceBusy}
        >
          Посчитать аудиторию
        </Button>
      </div>
    </div>
  );
}
function Buttons({ model, disabled }) {
  const buttons = model.buttons || [];
  return (
    <div className="rn-broadcast-buttons">
      <Group justify="space-between">
        <Switch
          label="Добавить кнопки"
          color="teal"
          checked={buttons.length > 0}
          disabled={disabled}
          onChange={(e) =>
            e.currentTarget.checked ? model.addButton() : model.clearButtons()
          }
        />
        {buttons.length > 0 && (
          <Button
            size="xs"
            variant="subtle"
            leftSection={<PiPlus />}
            disabled={disabled || buttons.length >= 8}
            onClick={() => model.addButton()}
          >
            Ещё кнопку
          </Button>
        )}
      </Group>
      {buttons.map((button, index) => (
        <section
          className="rn-broadcast-button-editor"
          key={button.id || index}
        >
          <Group justify="space-between">
            <strong>Кнопка {index + 1}</strong>
            <Tooltip label="Удалить кнопку">
              <ActionIcon
                aria-label={`Удалить кнопку ${index + 1}`}
                variant="subtle"
                color="red"
                disabled={disabled}
                onClick={() => model.removeButton(index)}
              >
                <PiTrash size={18} />
              </ActionIcon>
            </Tooltip>
          </Group>
          <div className="rn-broadcast-button-fields">
            <TextInput
              label="Текст кнопки"
              placeholder="Например: Получить скидку"
              value={button.text || ""}
              maxLength={256}
              disabled={disabled}
              onChange={(e) =>
                model.updateButton(index, "text", e.target.value)
              }
            />
            <Select
              label="Назначение"
              value={button.type || "url"}
              data={BROADCAST_BUTTON_TYPES}
              disabled={disabled}
              onChange={(value) =>
                model.updateButton(index, "type", value || "url")
              }
            />
            <Select
              label="Цвет кнопки"
              data={BROADCAST_STYLES}
              value={button.style || ""}
              disabled={disabled}
              onChange={(value) =>
                model.updateButton(index, "style", value || "")
              }
            />
          </div>
          {button.type === "url" && (
            <TextInput
              label="Ссылка"
              placeholder="https://t.me/… или https://…"
              type="url"
              value={button.url || ""}
              maxLength={256}
              disabled={disabled}
              onChange={(e) => model.updateButton(index, "url", e.target.value)}
            />
          )}
          {button.type === "promo" && (
            <TextInput
              label="Промокод"
              placeholder="LINK20"
              value={button.promoCode || ""}
              maxLength={256}
              disabled={disabled}
              onChange={(e) =>
                model.updateButton(index, "promoCode", e.target.value)
              }
            />
          )}
          <Accordion
            variant="separated"
            mt="xs"
            className="rn-broadcast-advanced"
          >
            <Accordion.Item value="emoji">
              <Accordion.Control>Premium emoji</Accordion.Control>
              <Accordion.Panel>
                <TextInput
                  label="ID premium emoji или полный tg-emoji код"
                  placeholder="5206222720416643915"
                  value={button.iconCustomEmojiId || ""}
                  maxLength={128}
                  disabled={disabled}
                  onChange={(e) =>
                    model.updateButton(
                      index,
                      "iconCustomEmojiId",
                      e.target.value,
                    )
                  }
                />
                <Text size="xs" c="dimmed" mt="xs">
                  Полный tg-emoji код также можно вставить в текст кнопки.
                </Text>
              </Accordion.Panel>
            </Accordion.Item>
          </Accordion>
        </section>
      ))}
      {buttons.length > 0 && (
        <Button
          variant="subtle"
          size="sm"
          data-action="admin-broadcast-save-buttons"
          disabled={disabled || !model.buttonsDirty}
          leftSection={<PiCheck />}
        >
          Сохранить кнопки
        </Button>
      )}
    </div>
  );
}
function Progress({ draft, hidden }) {
  if (!draft || !draft.recipientCount) return null;
  const m = broadcastMetrics(draft);
  return (
    <div className="rn-broadcast-progress">
      <Group justify="space-between">
        <span>{BROADCAST_STATUS[draft.status] || draft.status}</span>
        <Private hidden={hidden}>{m.percent}%</Private>
      </Group>
      <i>
        <b style={{ width: `${m.percent}%` }} />
      </i>
      <small>
        <Private hidden={hidden}>
          {number(m.sent)} отправлено · {number(m.failed)} ошибок ·{" "}
          {number(m.total)} получателей
        </Private>
      </small>
    </div>
  );
}
function Doodle() {
  const id = useId().replace(/:/g, "");
  return (
    <svg className="rn-broadcast-doodle" aria-hidden="true">
      <defs>
        <pattern id={id} width="185" height="175" patternUnits="userSpaceOnUse">
          <g fill="none" stroke="currentColor" strokeWidth="1.3">
            <path d="m18 34 42-12-12 40-10-15-20-13Zm20 13 22-25M98 18l5 10 11 2-8 8 2 11-10-5-10 5 2-11-8-8 11-2 5-10Z" />
            <rect x="92" y="95" width="54" height="36" rx="11" />
            <path d="m108 131-8 10 20-10M105 107h26M105 116h17" />
            <circle cx="33" cy="111" r="24" />
            <path d="M22 119q11 12 22 0M24 104h1M41 104h1M158 58q-19-15-19 0t19 20q19-15 19-20t-19 0Z" />
          </g>
        </pattern>
      </defs>
      <rect width="100%" height="100%" fill={`url(#${id})`} />
    </svg>
  );
}
function Preview({ model, brand, hidden }) {
  const email = model.channel === "email",
    draft = email ? model.email?.draft : model.telegram;
  const Icon =
    {
      photo: PiImage,
      video: PiVideo,
      audio: PiMusicNote,
      document: PiFileText,
    }[draft?.sourceKind] || PiFileText;
  const media =
    !email && draft?.sourceKind && !["text", "html"].includes(draft.sourceKind);
  if (!model.canBroadcast)
    return (
      <section className="rn-broadcast-card rn-broadcast-preview">
        <h2>Почтовое подключение</h2>
        <div className="rn-broadcast-preview-scene">
          <div className="rn-email-preview-sheet">
            <PiEnvelopeSimple size={38} />
            <h3>
              {model.smtp?.draft?.enabled
                ? "Отправка писем включена"
                : "Отправка писем выключена"}
            </h3>
            <p>
              Сохраните параметры SMTP, проверьте подключение и отправьте
              тестовое письмо.
            </p>
          </div>
        </div>
      </section>
    );
  return (
    <section
      className={`rn-broadcast-card rn-broadcast-preview ${email ? "is-email" : "is-telegram"}`}
    >
      <h2>Предпросмотр</h2>
      <div className="rn-broadcast-preview-scene">
        <Doodle />
        {email ? (
          <div className="rn-email-preview-sheet">
            <header>
              <div className="rn-email-avatar">
                <PiEnvelopeSimple size={22} />
              </div>
              <div>
                <strong>{brand}</strong>
                <small>
                  {model.smtp?.draft?.from || "Адрес отправителя из SMTP"}
                </small>
              </div>
            </header>
            <h3>{model.subject || "Тема письма"}</h3>
            <div>
              {model.body || "Здесь появится текст письма из редактора."}
            </div>
            <footer>Текстовое письмо · без HTML-разметки</footer>
          </div>
        ) : (
          <div className="rn-telegram-message">
            <strong>{brand}</strong>
            {media && (
              <div className="rn-telegram-media">
                <Icon size={42} />
                <span>
                  {{
                    photo: "Фото",
                    video: "Видео",
                    audio: "Аудио",
                    voice: "Голосовое сообщение",
                    animation: "Анимация",
                    document: "Файл",
                  }[draft.sourceKind] || "Вложение"}
                </span>
              </div>
            )}
            <p>
              {draft?.sourceText ||
                draft?.sourcePreview ||
                "После создания сообщения в боте здесь появится его предпросмотр."}
            </p>
            {(model.buttons || []).map((b, index) => (
              <div
                className="rn-telegram-button"
                data-style={b.style || "default"}
                key={b.id || index}
              >
                {broadcastButtonText(b.text) || "Текст кнопки"}
              </div>
            ))}
            <small>
              {draft?.updatedAt
                ? new Date(draft.updatedAt).toLocaleTimeString("ru", {
                    hour: "2-digit",
                    minute: "2-digit",
                    timeZone: "Europe/Moscow",
                  })
                : "Черновик"}
            </small>
          </div>
        )}
      </div>
      <footer className="rn-preview-note">
        {email
          ? "Проверьте реальную доставку тестовым письмом перед запуском."
          : "Оформление и вложения сохраняются при отправке. Полный предпросмотр — проверка в Telegram."}
      </footer>
    </section>
  );
}
function Templates({ model, channel, save, onClose }) {
  const [items, setItems] = useState([]),
    [name, setName] = useState(""),
    [error, setError] = useState(""),
    [busy, setBusy] = useState(false),
    [deleting, setDeleting] = useState(null);
  const load = async () => {
    setBusy(true);
    try {
      setItems((await model.templates("list")) || []);
    } catch (e) {
      setError(e.message);
    } finally {
      setBusy(false);
    }
  };
  useLayoutEffect(() => {
    if (!save) void load();
  }, []);
  const run = async (action, args) => {
    setBusy(true);
    setError("");
    try {
      await model.templates(action, args);
      if (action === "delete") {
        setDeleting(null);
        await load();
      } else onClose();
    } catch (e) {
      setError(e.message);
    } finally {
      setBusy(false);
    }
  };
  return (
    <Modal
      opened
      onClose={onClose}
      title={save ? "Сохранить как шаблон" : "Шаблоны рассылки"}
      size={600}
      closeButtonProps={{ "aria-label": "Закрыть шаблоны" }}
      className="rn-broadcast-modal"
    >
      <Stack>
        {save ? (
          <>
            <Text size="sm" c="dimmed">
              {channel === "email"
                ? "Сохраним тему, текст и отбор получателей."
                : "Сохраним сообщение, кнопки и отбор получателей. Применение шаблона не запускает рассылку."}
            </Text>
            <TextInput
              label="Название шаблона"
              maxLength={80}
              value={name}
              onChange={(e) => setName(e.target.value)}
              autoFocus
            />
            <Button
              disabled={!name.trim() || busy}
              loading={busy}
              onClick={() => run("save", { name })}
            >
              Сохранить шаблон
            </Button>
          </>
        ) : (
          <>
            {items
              .filter((t) => t.channel === channel)
              .map((t) => (
                <Group
                  key={t.id}
                  justify="space-between"
                  className="rn-broadcast-template"
                >
                  <div>
                    <strong>{t.name}</strong>
                    <Text size="xs" c="dimmed">
                      {new Date(t.createdAt).toLocaleDateString("ru")}
                    </Text>
                  </div>
                  <Group gap="xs">
                    <Button
                      variant="light"
                      disabled={busy}
                      onClick={() => run("apply", { id: t.id })}
                    >
                      Применить
                    </Button>
                    <ActionIcon
                      color="red"
                      variant="subtle"
                      aria-label={`Удалить шаблон ${t.name}`}
                      disabled={busy}
                      onClick={() => setDeleting(t)}
                    >
                      <PiTrash />
                    </ActionIcon>
                  </Group>
                </Group>
              ))}
            {!items.some((t) => t.channel === channel) && !busy && (
              <Text c="dimmed">Шаблонов для этой вкладки пока нет.</Text>
            )}
            {busy && <Text size="sm">Загрузка…</Text>}
            {deleting && (
              <Alert color="red" title={`Удалить «${deleting.name}»?`}>
                <Group mt="sm">
                  <Button
                    size="xs"
                    color="red"
                    onClick={() => run("delete", { id: deleting.id })}
                    disabled={busy}
                  >
                    Удалить
                  </Button>
                  <Button
                    size="xs"
                    variant="default"
                    onClick={() => setDeleting(null)}
                  >
                    Отмена
                  </Button>
                </Group>
              </Alert>
            )}
          </>
        )}
        {error && <Alert color="red">{error}</Alert>}
      </Stack>
    </Modal>
  );
}
export function Broadcast({ model: root, dispatch, hidden, Legacy }) {
  const model = root.broadcastConsole || {},
    channel = model.channel || "telegram",
    email = channel === "email",
    draft = email ? model.email?.draft : model.telegram;
  const [templates, setTemplates] = useState(null);
  const mounted = useRef(false);
  useLayoutEffect(() => {
    if (mounted.current)
      return animateAdminEntrance(
        document.querySelector(".rn-broadcast-section"),
      );
    mounted.current = true;
  }, [channel]);
  const smtpNode = useMemo(() => {
    const t = document.createElement("template");
    t.innerHTML = model.smtpHTML || "";
    return t.content.firstElementChild;
  }, [model.smtpHTML]);
  const running = draft?.status === "running",
    busy = Boolean(email ? model.emailBusy : model.telegramBusy),
    disabled = running || busy || !model.canBroadcast;
  const hasMessage = email
    ? model.subject?.trim().length >= 2 && model.body?.trim().length >= 2
    : Boolean(model.telegram?.sourceKind);
  const pane = email
    ? model.canBroadcast
      ? model.emailEditorTab || "message"
      : model.emailEditorTab === "test"
        ? "test"
        : "connection"
    : "message";
  return (
    <div className="rn-broadcast-section">
      {model.error && (
        <Alert color="red">
          {model.error}
          <Button variant="subtle" data-action="refresh">
            Повторить
          </Button>
        </Alert>
      )}
      <div className="rn-broadcast-tabs">
        <nav aria-label="Тип рассылки">
          {model.canBroadcast && (
            <button
              role="tab"
              aria-selected={!email}
              className={!email ? "is-active" : ""}
              data-action="admin-broadcast-tab"
              data-value="telegram"
            >
              <PiTelegramLogo size={19} />
              Telegram
            </button>
          )}
          <button
            role="tab"
            aria-selected={email}
            className={email ? "is-active" : ""}
            data-action="admin-broadcast-tab"
            data-value="email"
          >
            <PiEnvelopeSimple size={19} />
            Рассылка по почте
          </button>
        </nav>
        {model.canBroadcast && (
          <Button
            variant="default"
            size="xs"
            leftSection={<PiBookmarkSimple />}
            disabled={disabled || !draft?.updatedAt}
            onClick={() => setTemplates("list")}
          >
            Шаблоны
          </Button>
        )}
      </div>
      {model.canBroadcast && (
        <Stats draft={draft} loaded={Boolean(draft)} hidden={hidden} />
      )}
      {email && (
        <Group className="rn-email-section-tabs" gap="xs">
          {[
            ...(model.canBroadcast ? [["message", "Письмо"]] : []),
            ...(model.canSMTP ? [["connection", "Подключение SMTP"]] : []),
            ["test", "Тестовая отправка"],
          ].map(([id, label]) => (
            <Button
              key={id}
              size="xs"
              variant={pane === id ? "light" : "subtle"}
              data-action="admin-email-editor-tab"
              data-value={id}
            >
              {label}
            </Button>
          ))}
        </Group>
      )}
      <div className="rn-broadcast-grid">
        <div className="rn-broadcast-editor-column">
          <section className="rn-broadcast-card rn-broadcast-editor">
            <div className="rn-broadcast-card-head">
              <h2>
                {email
                  ? pane === "connection"
                    ? "Подключение почты"
                    : pane === "test"
                      ? "Проверка доставки"
                      : "Новое письмо"
                  : "Новое сообщение"}
              </h2>
              {model.canBroadcast && (
                <Badge color={running ? "teal" : "gray"} variant="light">
                  {BROADCAST_STATUS[draft?.status] || "Загрузка…"}
                </Badge>
              )}
            </div>
            {email && pane === "connection" ? (
              <div className="rn-smtp-pane">
                <Legacy node={smtpNode} />
              </div>
            ) : email && pane === "test" ? (
              <div className="rn-email-test-pane">
                <Text size="sm" c="dimmed">
                  {model.canBroadcast
                    ? "Отправьте себе письмо из редактора и проверьте оформление и доставку."
                    : "Отправьте тестовое письмо для проверки SMTP."}
                </Text>
                <TextInput
                  type="email"
                  label="Получатель тестового письма"
                  placeholder="you@example.com"
                  defaultValue={model.previewAddress}
                  onChange={(e) => model.updatePreviewAddress(e.target.value)}
                  disabled={busy}
                />
                <Button
                  data-action="admin-email-test"
                  leftSection={<PiEnvelopeSimple />}
                  disabled={
                    busy ||
                    model.smtp?.busy ||
                    (model.canBroadcast && !model.email?.configured)
                  }
                  loading={
                    model.emailBusy === "preview" || model.smtp?.busy === "test"
                  }
                >
                  Отправить тестовое письмо
                </Button>
                {model.smtp?.error && (
                  <Alert color="red">{model.smtp.error}</Alert>
                )}
              </div>
            ) : (
              <>
                <Audience
                  key={channel}
                  model={model}
                  disabled={disabled || model.audienceBusy}
                  hidden={hidden}
                />
                {email ? (
                  <div className="rn-email-fields">
                    <TextInput
                      label="Тема письма"
                      value={model.subject || ""}
                      onChange={(e) =>
                        model.updateEmail("subject", e.target.value)
                      }
                      maxLength={160}
                      placeholder="Тема, которую увидят получатели"
                      disabled={disabled}
                    />
                    <Textarea
                      label="Текст письма"
                      value={model.body || ""}
                      onChange={(e) =>
                        model.updateEmail("body", e.target.value)
                      }
                      placeholder="Напишите сообщение…"
                      minRows={9}
                      maxRows={16}
                      autosize
                      maxLength={20000}
                      disabled={disabled}
                    />
                    <Group justify="space-between">
                      <Text size="xs" c="dimmed">
                        Текстовое письмо · {model.body?.length || 0}/20 000
                      </Text>
                      <Button
                        variant="subtle"
                        size="sm"
                        data-action="admin-email-save"
                        disabled={disabled || !hasMessage || !model.emailDirty}
                      >
                        Сохранить письмо
                      </Button>
                    </Group>
                  </div>
                ) : (
                  <>
                    <div className="rn-broadcast-capture">
                      <div className="rn-broadcast-capture-icon">
                        <PiTelegramLogo size={35} />
                      </div>
                      <h3>
                        {model.telegram?.sourceKind
                          ? "Сообщение получено из Telegram"
                          : "Сообщение создаётся в Telegram-боте"}
                      </h3>
                      <p>
                        {model.telegram?.status === "awaiting_message"
                          ? "Отправьте следующее сообщение боту. Оно появится здесь автоматически."
                          : model.telegram?.sourceKind
                            ? model.telegram.sourcePreview
                            : "Нажмите кнопку, отправьте боту текст, фото, видео или файл и вернитесь сюда для настройки и отправки."}
                      </p>
                      <Button
                        color="teal"
                        size="md"
                        leftSection={<PiTelegramLogo size={20} />}
                        data-action="admin-broadcast-capture"
                        disabled={disabled}
                        loading={model.telegramBusy === "capture"}
                      >
                        {model.telegram?.sourceKind
                          ? "Заменить сообщение в боте"
                          : "Ввести сообщение в боте"}
                      </Button>
                    </div>
                    <Buttons model={model} disabled={disabled} />
                  </>
                )}
                <Accordion
                  variant="separated"
                  className="rn-broadcast-advanced"
                >
                  <Accordion.Item value="delivery">
                    <Accordion.Control icon={<PiGear size={18} />}>
                      Дополнительные настройки
                    </Accordion.Control>
                    <Accordion.Panel>
                      <Group wrap="wrap">
                        <Button
                          variant="default"
                          leftSection={<PiEye />}
                          data-action={
                            email
                              ? "admin-email-editor-tab"
                              : "admin-broadcast-preview"
                          }
                          data-value={email ? "test" : undefined}
                          disabled={disabled || !hasMessage}
                        >
                          {email
                            ? "Отправить себе для проверки"
                            : "Проверить в Telegram"}
                        </Button>
                        {!email && (
                          <Button
                            variant="subtle"
                            color="red"
                            data-action="admin-broadcast-reset"
                            disabled={disabled}
                          >
                            Очистить черновик
                          </Button>
                        )}
                      </Group>
                      <Text size="xs" c="dimmed" mt="sm">
                        Перед запуском покажем число получателей и попросим
                        подтвердить отправку.
                      </Text>
                    </Accordion.Panel>
                  </Accordion.Item>
                </Accordion>
              </>
            )}
            {draft?.lastError && (
              <Alert color="red" mt="md">
                {draft.lastError}
              </Alert>
            )}
            <Progress draft={draft} hidden={hidden} />
          </section>
          {model.canBroadcast && pane === "message" && (
            <div className="rn-broadcast-send-actions">
              <Button
                color="teal"
                size="md"
                leftSection={
                  email ? (
                    <PiEnvelopeSimple size={20} />
                  ) : (
                    <PiTelegramLogo size={20} />
                  )
                }
                data-action={
                  email
                    ? "admin-email-open-confirm"
                    : "admin-broadcast-open-confirm"
                }
                disabled={
                  disabled || !hasMessage || (email && !model.email?.configured)
                }
                loading={model.audienceBusy || busy}
              >
                {running ? "Рассылка идёт…" : "Отправить рассылку"}
              </Button>
              <Button
                variant="default"
                size="md"
                leftSection={<PiBookmarkSimple size={20} />}
                onClick={() => setTemplates("save")}
                disabled={disabled || !hasMessage}
              >
                Сохранить как шаблон
              </Button>
            </div>
          )}
          {email && model.canBroadcast && !model.email?.configured && (
            <Alert color="yellow">
              Для отправки писем настройте SMTP во вкладке «Подключение SMTP».
            </Alert>
          )}
        </div>
        <Preview model={model} brand={root.brand} hidden={hidden} />
      </div>
      {templates && (
        <Templates
          key={`${channel}:${templates}`}
          model={model}
          channel={channel}
          save={templates === "save"}
          onClose={() => setTemplates(null)}
        />
      )}
    </div>
  );
}

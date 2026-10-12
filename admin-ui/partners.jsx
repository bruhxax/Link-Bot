import React, { useMemo, useState } from "react";
import {
  ActionIcon,
  Alert,
  Avatar,
  Button,
  Checkbox,
  CopyButton,
  Group,
  Menu,
  Modal,
  NumberInput,
  Select,
  Stack,
  Text,
  TextInput,
  Tooltip,
} from "@mantine/core";
import { DatePickerInput } from "@mantine/dates";
import "dayjs/locale/ru.js";
import {
  PiUsersDuotone,
  PiUserDuotone,
  PiFileTextDuotone,
  PiCalendarBlank,
  PiCaretDown,
  PiDotsThreeVertical,
  PiCaretRight,
  PiArrowsClockwise,
  PiMagnifyingGlass,
  PiPlusCircle,
  PiPercent,
  PiUser,
  PiCopy,
  PiCheck,
  PiCaretUpDown,
} from "react-icons/pi";
import {
  applicationStatuses,
  partnerRows,
  partnerMetrics,
  partnersMonthRange,
} from "./partners-data.mjs";
import "./partners.css";

const number = (value) => Number(value || 0).toLocaleString("ru");
const date = (value) =>
  value
    ? new Date(value).toLocaleString("ru", {
        timeZone: "Europe/Moscow",
        day: "numeric",
        month: "short",
        year: "numeric",
        hour: "2-digit",
        minute: "2-digit",
      })
    : "—";
const money = (value, currency = "RUB") =>
  new Intl.NumberFormat("ru", {
    style: "currency",
    currency: currency === "RUB" ? "RUB" : "USD",
    maximumFractionDigits: 2,
  }).format(value || 0);
function Private({ children, hidden }) {
  return (
    <span className="rn-private-value" aria-hidden={hidden}>
      {children}
    </span>
  );
}
function Person({ item, hidden }) {
  return (
    <div className="rn-partner-person">
      <Avatar src={item.avatarUrl || undefined} radius={8} size={43}>
        {(item.username || "П").slice(0, 2).toUpperCase()}
      </Avatar>
      <div>
        <strong>
          {item.username
            ? `@${item.username.replace(/^@/, "")}`
            : "Пользователь"}
        </strong>
        <small>
          ID: <Private hidden={hidden}>{item.telegramId}</Private>
        </small>
      </div>
    </div>
  );
}
function Status({ item, applications }) {
  const status = applications
    ? item.status
    : item.isActive
      ? "active"
      : "disabled";
  const label = applications
    ? applicationStatuses.find((s) => s.value === status)?.label || "Неизвестно"
    : status === "active"
      ? "Активен"
      : "Отключён";
  return (
    <span className="rn-partner-status" data-status={status}>
      <i />
      {label}
    </span>
  );
}
function Metric({ title, value, percent, color, Icon, hidden, loaded }) {
  return (
    <div className="rn-partner-metric" style={{ "--partner-color": color }}>
      <div className="rn-partner-icon">
        <Icon size={32} />
      </div>
      <div>
        <span>{title}</span>
        <strong>
          <Private hidden={hidden}>{loaded ? number(value) : "—"}</Private>
        </strong>
        <div className="rn-partner-metric-progress">
          <i>
            <b style={{ width: `${Math.max(0, Math.min(100, percent))}%` }} />
          </i>
          <small>
            <Private hidden={hidden}>
              {loaded ? `${Math.round(percent)}%` : "—"}
            </Private>
          </small>
        </div>
      </div>
    </div>
  );
}

function PartnersTable({ model, items, applications, range, hidden, onOpen }) {
  const [query, setQuery] = useState(""),
    [status, setStatus] = useState(""),
    [descending, setDescending] = useState(true),
    [selected, setSelected] = useState(new Set());
  const rows = useMemo(
    () =>
      partnerRows(items, { query, status, range, applications, descending }),
    [items, query, status, range, applications, descending],
  );
  const checked =
    rows.length > 0 && rows.every((item) => selected.has(item.id));
  const toggle = (id, checked) =>
    setSelected((previous) => {
      const next = new Set(previous);
      checked ? next.add(id) : next.delete(id);
      return next;
    });
  const title = applications ? "Заявки" : "Партнёры",
    Icon = applications ? PiFileTextDuotone : PiUsersDuotone;
  return (
    <section
      className="rn-partner-card rn-partners-table"
      data-kind={applications ? "applications" : "partners"}
    >
      <div className="rn-partner-section-head">
        <div
          className="rn-partner-icon"
          style={{ "--partner-color": applications ? "#fb5575" : "#05cea0" }}
        >
          <Icon size={29} />
        </div>
        <div className="rn-partner-heading">
          <h2>
            {title}
            <span className="rn-partner-count">
              <Private hidden={hidden}>
                {number(partnerRows(items, { range, applications }).length)}
              </Private>
            </span>
          </h2>
          <p>
            {applications
              ? "Заявки от партнёров и решения по ним"
              : "Партнёры, получающие процент отчислений"}
          </p>
        </div>
        <div className="rn-partner-table-tools">
          {!applications && (
            <TextInput
              aria-label="Поиск партнёров"
              placeholder="Поиск по username или ID…"
              leftSection={<PiMagnifyingGlass size={19} />}
              value={query}
              onChange={(event) => setQuery(event.target.value)}
            />
          )}
          <Select
            aria-label={`Статус ${applications ? "заявки" : "партнёра"}`}
            placeholder="Статус"
            clearable
            value={status || null}
            onChange={(value) => setStatus(value || "")}
            rightSection={<PiCaretDown />}
            data={
              applications
                ? applicationStatuses
                : [
                    { value: "active", label: "Активен" },
                    { value: "disabled", label: "Отключён" },
                  ]
            }
          />
          <Tooltip label={`Обновить ${title.toLowerCase()}`}>
            <ActionIcon
              aria-label={`Обновить ${title.toLowerCase()}`}
              onClick={() => model.refreshPartners()}
              disabled={Boolean(model.partnersBusy)}
            >
              <PiArrowsClockwise size={22} />
            </ActionIcon>
          </Tooltip>
        </div>
      </div>
      <div className="rn-partner-table-scroll">
        <table>
          <thead>
            <tr>
              <th>
                <Checkbox
                  size="xs"
                  aria-label={`Выбрать все: ${title.toLowerCase()}`}
                  checked={checked}
                  indeterminate={
                    !checked && rows.some((item) => selected.has(item.id))
                  }
                  onChange={(event) =>
                    setSelected((previous) => {
                      const next = new Set(previous);
                      for (const item of rows)
                        event.target.checked
                          ? next.add(item.id)
                          : next.delete(item.id);
                      return next;
                    })
                  }
                />
              </th>
              <th>Пользователь</th>
              <th>Telegram ID</th>
              <th>Процент, %</th>
              <th>
                <button
                  onClick={() => setDescending(!descending)}
                  aria-label={`Сортировка ${applications ? "заявок" : "партнёров"} по дате`}
                  aria-pressed={!descending}
                >
                  {applications ? "Дата заявки" : "Дата добавления"}
                  <PiCaretUpDown size={15} />
                </button>
              </th>
              <th>Статус</th>
              <th>Действия</th>
            </tr>
          </thead>
          <tbody>
            {rows.map((item) => (
              <tr
                key={item.id}
                tabIndex={0}
                aria-label={`Открыть ${applications ? "заявку" : "партнёра"} ${item.username || "пользователя"}`}
                onClick={(event) => {
                  if (
                    !event.target.closest("input,button,label,[role=checkbox]")
                  )
                    onOpen(item);
                }}
                onKeyDown={(event) => {
                  if (
                    event.target === event.currentTarget &&
                    ["Enter", " "].includes(event.key)
                  ) {
                    event.preventDefault();
                    onOpen(item);
                  }
                }}
              >
                <td>
                  <Checkbox
                    size="xs"
                    aria-label={`Выбрать ${item.username || "пользователя"}`}
                    checked={selected.has(item.id)}
                    onChange={(event) => toggle(item.id, event.target.checked)}
                  />
                </td>
                <td>
                  <Person item={item} hidden={hidden} />
                </td>
                <td className="rn-partner-telegram">
                  <Private hidden={hidden}>{item.telegramId}</Private>
                </td>
                <td>
                  <Private hidden={hidden}>
                    {applications
                      ? item.requestedPercent
                      : item.commissionPercent}
                    %
                  </Private>
                </td>
                <td>
                  <span className="rn-partner-date">
                    <PiCalendarBlank size={18} />
                    {date(item.createdAt)}
                  </span>
                </td>
                <td>
                  <Status item={item} applications={applications} />
                </td>
                <td>
                  <ActionIcon
                    aria-label={`Открыть ${applications ? "заявку" : "партнёра"} ${item.username || "пользователя"}`}
                    onClick={() => onOpen(item)}
                  >
                    <PiCaretRight size={22} />
                  </ActionIcon>
                </td>
              </tr>
            ))}
            {!rows.length && (
              <tr>
                <td className="rn-partner-empty" colSpan={7}>
                  {model.partnersLoaded
                    ? query || status
                      ? "По выбранным фильтрам ничего не найдено"
                      : `${applications ? "Заявок" : "Партнёров"} за выбранный период нет`
                    : "Загрузка партнёров…"}
                </td>
              </tr>
            )}
          </tbody>
        </table>
      </div>
    </section>
  );
}

function PartnerDetails({ model, item, applications, hidden, onClose }) {
  const [percent, setPercent] = useState(
      applications ? item.requestedPercent : item.commissionPercent,
    ),
    [error, setError] = useState("");
  const busy = Boolean(model.partnersBusy),
    valid = Number.isInteger(percent) && percent >= 0 && percent <= 100;
  const run = async (action) => {
    setError("");
    try {
      await action();
    } catch (error) {
      setError(error.message || "Не удалось сохранить изменения");
    }
  };
  const stats = item.stats || {},
    link = applications ? item.resourceUrl : item.inviteUrl;
  return (
    <Modal
      opened
      onClose={onClose}
      title={applications ? "Заявка партнёра" : "Партнёр"}
      size={650}
      className="rn-partner-modal"
      closeButtonProps={{ "aria-label": "Закрыть карточку партнёра" }}
      overlayProps={{ backgroundOpacity: 0.65 }}
    >
      <Stack gap="lg">
        <Group justify="space-between">
          <Person item={item} hidden={hidden} />
          <Status item={item} applications={applications} />
        </Group>
        <Text size="sm" c="dimmed">
          {applications ? "Заявка от" : "Добавлен"}: {date(item.createdAt)}
        </Text>
        {link && (
          <div className="rn-partner-link">
            <Text size="sm" c="dimmed">
              {applications ? "Ресурс" : "Партнёрская ссылка"}
            </Text>
            <Group wrap="nowrap">
              <a
                href={/^https?:\/\//i.test(link) ? link : undefined}
                target="_blank"
                rel="noopener noreferrer"
              >
                {link}
              </a>
              <CopyButton value={link}>
                {({ copied, copy }) => (
                  <ActionIcon onClick={copy} aria-label="Скопировать ссылку">
                    {copied ? <PiCheck /> : <PiCopy />}
                  </ActionIcon>
                )}
              </CopyButton>
            </Group>
          </div>
        )}
        <div className="rn-partner-detail-metrics">
          {(applications
            ? [
                ["Запрошено", `${item.requestedPercent}%`],
                ["Аудитория в месяц", number(item.expectedMonthlyUsers)],
              ]
            : [
                ["Пришло", number(stats.visitors)],
                ["Триалы", number(stats.trialUsers)],
                ["Купили", number(stats.payingUsers)],
                ["Оборот", money(stats.revenue)],
                [
                  "Начислено",
                  stats.currency === "STARS"
                    ? `${number(stats.commission)} ★`
                    : money(stats.commission, stats.currency),
                ],
              ]
          ).map(([label, value]) => (
            <div key={label}>
              <span>{label}</span>
              <strong>
                <Private hidden={hidden}>{value}</Private>
              </strong>
            </div>
          ))}
        </div>
        {(!applications || item.status === "pending") && (
          <NumberInput
            label="Процент отчислений, %"
            value={percent}
            onChange={setPercent}
            min={0}
            max={100}
            allowDecimal={false}
            disabled={busy || !model.canManagePartners}
            leftSection={<PiPercent />}
          />
        )}
        {error && <Alert color="red">{error}</Alert>}
        {model.canManagePartners && (
          <Group justify="flex-end">
            {applications ? (
              item.status === "pending" && (
                <>
                  <Button
                    color="red"
                    variant="light"
                    disabled={busy}
                    onClick={() =>
                      run(() => model.reviewPartner(item.id, false, 0))
                    }
                  >
                    Отклонить
                  </Button>
                  <Button
                    color="teal"
                    disabled={busy || !valid}
                    loading={model.partnersBusy === `review-${item.id}`}
                    onClick={() =>
                      run(() => model.reviewPartner(item.id, true, percent))
                    }
                  >
                    Принять заявку
                  </Button>
                </>
              )
            ) : (
              <>
                <Button
                  variant="default"
                  disabled={busy}
                  onClick={() =>
                    run(() =>
                      model.updatePartner(item.id, { active: !item.isActive }),
                    )
                  }
                >
                  {item.isActive ? "Отключить" : "Включить"}
                </Button>
                <Button
                  disabled={busy || !valid}
                  loading={model.partnersBusy === `update-${item.id}`}
                  onClick={() =>
                    run(() => model.updatePartner(item.id, { percent }))
                  }
                >
                  Сохранить процент
                </Button>
              </>
            )}
          </Group>
        )}
      </Stack>
    </Modal>
  );
}

export function Partners({ model, hidden }) {
  const [range, setRange] = useState(partnersMonthRange),
    [telegram, setTelegram] = useState(""),
    [percent, setPercent] = useState(20),
    [error, setError] = useState(""),
    [opened, setOpened] = useState(null);
  const data = model.partners || { partners: [], applications: [] },
    metrics = partnerMetrics(data, range),
    busy = Boolean(model.partnersBusy);
  const selected =
    opened &&
    (opened.applications ? data.applications : data.partners).find(
      (item) => item.id === opened.id,
    );
  const valid =
    Number.isSafeInteger(Number(telegram)) &&
    Number(telegram) > 0 &&
    Number.isInteger(percent) &&
    percent >= 0 &&
    percent <= 100;
  const create = async (event) => {
    event.preventDefault();
    if (!valid || busy) return;
    setError("");
    try {
      await model.createPartner(Number(telegram), percent);
      setTelegram("");
    } catch (error) {
      setError(error.message || "Не удалось добавить партнёра");
    }
  };
  return (
    <div className="rn-partners-section" aria-busy={busy}>
      <div className="rn-partners-period">
        <DatePickerInput
          type="range"
          value={range}
          onChange={setRange}
          clearable
          locale="ru"
          valueFormat="D MMM YYYY"
          leftSection={<PiCalendarBlank size={20} />}
          rightSection={<PiCaretDown />}
          aria-label="Период партнёров"
          placeholder="Весь период"
        />
        <Menu position="bottom-end" withinPortal>
          <Menu.Target>
            <ActionIcon
              aria-label="Действия раздела партнёров"
              variant="subtle"
            >
              <PiDotsThreeVertical size={21} />
            </ActionIcon>
          </Menu.Target>
          <Menu.Dropdown>
            <Menu.Item onClick={() => setRange([null, null])}>
              Весь период
            </Menu.Item>
            <Menu.Item onClick={() => setRange(partnersMonthRange())}>
              Текущий месяц
            </Menu.Item>
            <Menu.Item disabled={busy} onClick={() => model.refreshPartners()}>
              Обновить партнёров
            </Menu.Item>
          </Menu.Dropdown>
        </Menu>
      </div>
      <div className="rn-partner-metrics">
        <Metric
          title="Всего партнёров"
          value={metrics.total}
          percent={metrics.totalShare}
          color="#278bff"
          Icon={PiUsersDuotone}
          hidden={hidden}
          loaded={model.partnersLoaded}
        />
        <Metric
          title="Активные"
          value={metrics.active}
          percent={metrics.activeShare}
          color="#00d0a0"
          Icon={PiUserDuotone}
          hidden={hidden}
          loaded={model.partnersLoaded}
        />
        <Metric
          title="Заявки"
          value={metrics.applications}
          percent={metrics.applicationsShare}
          color="#fa5278"
          Icon={PiFileTextDuotone}
          hidden={hidden}
          loaded={model.partnersLoaded}
        />
      </div>
      {model.partnersError && (
        <Alert color="red" title="Не удалось обновить партнёров">
          {model.partnersError}
          <Button variant="subtle" onClick={() => model.refreshPartners()}>
            Повторить
          </Button>
        </Alert>
      )}
      {model.canManagePartners && (
        <section className="rn-partner-card rn-partner-create">
          <div
            className="rn-partner-icon"
            style={{ "--partner-color": "#278bff" }}
          >
            <PiUser size={30} />
          </div>
          <div className="rn-partner-create-content">
            <h2>Добавить партнёра</h2>
            <p>
              Добавьте нового партнёра по Telegram ID и укажите процент
              отчислений
            </p>
            <form onSubmit={create}>
              <TextInput
                label="Telegram ID"
                placeholder="123456789"
                inputMode="numeric"
                maxLength={16}
                leftSection={<PiUser size={22} />}
                value={telegram}
                onChange={(event) =>
                  setTelegram(event.target.value.replace(/\D/g, ""))
                }
                disabled={busy}
              />
              <NumberInput
                label="Процент, %"
                value={percent}
                onChange={setPercent}
                min={0}
                max={100}
                allowDecimal={false}
                leftSection={<PiPercent size={22} />}
                disabled={busy}
              />
              <Button
                type="submit"
                leftSection={<PiPlusCircle size={22} />}
                disabled={!valid || busy}
                loading={model.partnersBusy === "create"}
              >
                Добавить
              </Button>
            </form>
            {error && (
              <Alert color="red" mt="sm">
                {error}
              </Alert>
            )}
          </div>
        </section>
      )}
      <PartnersTable
        model={model}
        items={data.applications || []}
        applications
        range={range}
        hidden={hidden}
        onOpen={(item) => setOpened({ id: item.id, applications: true })}
      />
      <PartnersTable
        model={model}
        items={data.partners || []}
        range={range}
        hidden={hidden}
        onOpen={(item) => setOpened({ id: item.id, applications: false })}
      />
      {selected && (
        <PartnerDetails
          key={`${opened.applications}:${selected.id}`}
          model={model}
          item={selected}
          applications={opened.applications}
          hidden={hidden}
          onClose={() => setOpened(null)}
        />
      )}
    </div>
  );
}

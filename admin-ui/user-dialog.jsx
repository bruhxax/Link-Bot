// Adapted from remnawave/frontend's user form, AGPL-3.0.
import React, { useState, useRef, useEffect } from "react";
import {
  Accordion,
  ActionIcon,
  Alert,
  Badge,
  Button,
  Card,
  Checkbox,
  CopyButton,
  Divider,
  Group,
  Input,
  Menu,
  Modal,
  NativeSelect,
  NumberInput,
  Paper,
  Progress,
  Select,
  SimpleGrid,
  Stack,
  Text,
  Textarea,
  TextInput,
  ThemeIcon,
  Title,
  Tooltip,
} from "@mantine/core";
import { DateTimePicker, getTimeRange } from "@mantine/dates";
import dayjs from "dayjs";
import "dayjs/locale/ru";
import {
  PiCalendarDuotone,
  PiClockDuotone,
  PiFloppyDiskDuotone,
  PiLinkDuotone,
} from "react-icons/pi";
import {
  TbChartLine,
  TbCheck,
  TbCopy,
  TbDevices2,
  TbDots,
  TbMail,
  TbSearch,
  TbSettings,
  TbShield,
  TbTag,
  TbUser,
  TbWebhook,
} from "react-icons/tb";
import {
  TbQrcode,
  TbJson,
  TbServerCog,
  TbTimeline,
  TbChartArcs,
} from "react-icons/tb";
import { renderSVG } from "../internal/miniapp/static/uqr.mjs";
import { TagInput } from "./tag-input.jsx";

function Section({
  title,
  icon: Icon,
  color,
  children,
  header,
  className = "",
}) {
  return (
    <Card
      className={`rn-user-section ${className}`}
      p="md"
      radius="md"
      shadow="none"
    >
      <Stack gap="md">
        {header || (
          <Group gap="sm">
            <ThemeIcon variant="soft" color={color} size={32}>
              <Icon size={20} />
            </ThemeIcon>
            <Title order={5}>{title}</Title>
          </Group>
        )}
        <Divider opacity={0.3} />
        {children}
      </Stack>
    </Card>
  );
}
function Copy({ value, label = "Копировать" }) {
  return (
    <CopyButton value={value || ""}>
      {({ copied, copy }) => (
        <Tooltip label={copied ? "Скопировано" : label} withinPortal={false}>
          <ActionIcon
            variant="subtle"
            color={copied ? "teal" : "gray"}
            onClick={copy}
            aria-label={label}
          >
            {copied ? <TbCheck size={18} /> : <TbCopy size={18} />}
          </ActionIcon>
        </Tooltip>
      )}
    </CopyButton>
  );
}
const units = ["KiB", "MiB", "GiB", "TiB", "PiB"];
function bestUnit(bytes) {
  return Math.min(
    4,
    Math.max(
      0,
      Math.floor(Math.log(Number(bytes) || 1073741824) / Math.log(1024)) - 1,
    ),
  );
}
export function TrafficInput({ value, onChange, disabled }) {
  const [unit, setUnit] = useState(() => bestUnit(value));
  const bytes = useRef(value);
  bytes.current = value;
  return (
    <Group align="flex-end" gap="xs" wrap="nowrap">
      <NumberInput
        label="Лимит трафика"
        description="0 — безлимит"
        leftSection={<TbChartLine size={16} />}
        style={{ flex: 1 }}
        value={Number(value || 0) / 1024 ** (unit + 1)}
        onChange={(v) => onChange(Math.round(Number(v) * 1024 ** (unit + 1)))}
        min={0}
        max={(1000000 * 1073741824) / 1024 ** (unit + 1)}
        allowNegative={false}
        hideControls
        thousandSeparator=","
        disabled={disabled}
      />
      <NativeSelect
        aria-label="Единица трафика"
        w={92}
        data={units}
        value={units[unit]}
        onChange={(e) => setUnit(units.indexOf(e.currentTarget.value))}
        disabled={disabled}
      />
    </Group>
  );
}
function Identification({ user, subscription, settings, squads }) {
  const [view, setView] = useState("");
  const limit = Number(subscription.trafficLimitBytes || 0),
    used = Number(subscription.usedTrafficBytes || 0);
  const status = user.isBlocked
    ? "DISABLED"
    : String(subscription.status || "none").toUpperCase();
  const statusColor =
    status === "ACTIVE"
      ? "teal"
      : status === "EXPIRED"
        ? "red"
        : status === "LIMITED"
          ? "yellow"
          : "gray";
  const expiry = dayjs(settings.expireAt || subscription.expiresAt);
  const daysLeft = expiry.diff(dayjs(), "day");
  const expiryColor =
    daysLeft <= 0 ? "red.5" : daysLeft <= 7 ? "yellow.4" : "teal.5";
  const expiryStyle =
    daysLeft <= 0
      ? { background: "rgba(239,68,68,.08)", borderColor: "rgba(239,68,68,.2)" }
      : daysLeft <= 7
        ? {
            background: "rgba(251,191,36,.10)",
            borderColor: "rgba(251,191,36,.2)",
          }
        : {
            background: "rgba(45,212,191,.08)",
            borderColor: "rgba(45,212,191,.2)",
          };
  const pretty = (bytes) => {
    if (!bytes) return "0 B";
    const u = bestUnit(bytes);
    return `${(bytes / 1024 ** (u + 1)).toFixed(2)} ${units[u]}`;
  };
  const id = subscription.panelId || user.customerId;
  const titles = {
    qr: "QR-код подписки",
    json: "Настройки пользователя · JSON",
    devices: "Устройства HWID",
    access: "Доступ пользователя",
    traffic: "Использование трафика",
    info: "Сведения о пользователе",
  };
  let qr = "";
  if (view === "qr" && subscription.subscriptionLink) {
    try {
      qr = renderSVG(subscription.subscriptionLink, {
        border: 4,
        pixelSize: 6,
        whiteColor: "#fff",
        blackColor: "#050607",
      });
    } catch {}
  }
  return (
    <>
      <Section
        className="rn-user-identity"
        header={
          <Group justify="space-between">
            <Group gap="sm">
              <ThemeIcon size={32} variant="soft" color={statusColor}>
                <TbUser size={20} />
              </ThemeIcon>
              <div>
                <Group gap={3}>
                  <Title order={5}>{id}</Title>
                  <Copy value={String(id)} />
                </Group>
                <Text size="xs" c="dimmed">
                  {subscription.panelUsername || user.username}
                </Text>
              </div>
            </Group>
            <Badge
              variant="light"
              radius="sm"
              color={statusColor}
              h={28}
              size="lg"
            >
              {status}
            </Badge>
          </Group>
        }
      >
        <Group
          gap={5}
          justify="center"
          wrap="nowrap"
          className="rn-user-toolbar"
        >
          {[
            ["qr", TbQrcode, "teal"],
            ["json", TbJson, "teal"],
            ["info", TbUser, "cyan"],
            ["access", TbServerCog, "cyan"],
            ["traffic", TbTimeline, "indigo"],
            ["devices", TbDevices2, "gray"],
          ].map(([key, Icon, color]) => (
            <Tooltip key={key} label={titles[key]} withinPortal={false}>
              <ActionIcon
                size="lg"
                variant="soft"
                color={color}
                aria-label={titles[key]}
                onClick={() => setView(key)}
                disabled={key === "qr" && !subscription.subscriptionLink}
              >
                <Icon size={22} />
              </ActionIcon>
            </Tooltip>
          ))}
          <Copy
            value={subscription.subscriptionLink}
            label="Копировать ссылку"
          />
        </Group>
        <Divider opacity={0.3} />
        <Stack gap="md">
          <Group justify="space-between">
            <Group gap={6}>
              <TbChartLine size={18} />
              <Text size="sm" className="rn-number">
                {pretty(used)}
              </Text>
            </Group>
            <Text size="sm" className="rn-number">
              {limit ? pretty(limit) : "∞"}
            </Text>
          </Group>
          <Progress
            size="sm"
            color={limit && used / limit > 0.95 ? "red" : "teal"}
            value={limit ? Math.min(100, (used / limit) * 100) : 0}
          />
          <SimpleGrid cols={2} spacing="xs">
            <Tooltip label="Дата окончания" withinPortal={false}>
              <Paper p="xs" withBorder style={expiryStyle}>
                <Group gap="xs" justify="center" wrap="nowrap" c={expiryColor}>
                  <PiCalendarDuotone size={18} />
                  <Text size="sm" fw={600}>
                    {expiry.isValid() ? expiry.format("DD.MM.YYYY HH:mm") : "—"}
                  </Text>
                </Group>
              </Paper>
            </Tooltip>
            <Tooltip
              label="Использовано трафика за всё время"
              withinPortal={false}
            >
              <Paper
                p="xs"
                radius="md"
                bd="1px solid rgba(99,102,241,.2)"
                bg="rgba(99,102,241,.08)"
              >
                <Group gap="xs" justify="center" c="indigo.5">
                  <TbChartArcs size={18} />
                  <Text size="sm" fw={600}>
                    {pretty(Number(subscription.lifetimeUsedTrafficBytes || 0))}
                  </Text>
                </Group>
              </Paper>
            </Tooltip>
          </SimpleGrid>
          <TextInput
            label="Ссылка на подписку"
            readOnly
            value={subscription.subscriptionLink || ""}
            placeholder="Ссылка на подписку недоступна"
            leftSection={<PiLinkDuotone size={16} />}
            rightSection={
              <Copy
                value={subscription.subscriptionLink}
                label="Копировать ссылку на подписку"
              />
            }
            aria-label="Ссылка на подписку"
          />
        </Stack>
      </Section>
      <Modal
        opened={Boolean(view)}
        onClose={() => setView("")}
        title={titles[view]}
        zIndex={340}
        size="lg"
        withinPortal
      >
        {view === "qr" && (
          <Stack align="center">
            {qr ? (
              <div
                className="rn-subscription-qr"
                role="img"
                aria-label="QR-код подписки"
                dangerouslySetInnerHTML={{ __html: qr }}
              />
            ) : (
              <Text c="dimmed">Ссылка недоступна</Text>
            )}
            <Text size="xs" style={{ overflowWrap: "anywhere" }}>
              {subscription.subscriptionLink}
            </Text>
            <Copy value={subscription.subscriptionLink} />
          </Stack>
        )}
        {view === "json" && (
          <pre className="rn-json-view">
            {JSON.stringify(
              {
                id: subscription.panelId,
                username: subscription.panelUsername,
                ...settings,
              },
              null,
              2,
            )}
          </pre>
        )}
        {view === "info" && (
          <Stack>
            <Text>ID в боте: {user.customerId}</Text>
            <Text>Telegram ID: {user.telegramId}</Text>
            <Text size="sm">
              Создан: {dayjs(user.createdAt).format("DD.MM.YYYY HH:mm")}
            </Text>
            <Text size="sm">Подписка: {subscription.name}</Text>
            <Badge color={statusColor} variant="light">
              {status}
            </Badge>
          </Stack>
        )}
        {view === "traffic" && (
          <Stack>
            <Text>Использовано: {pretty(used)}</Text>
            <Text>Лимит: {limit ? pretty(limit) : "∞"}</Text>
            <Progress value={limit ? Math.min(100, (used / limit) * 100) : 0} />
            <Text size="sm" c="dimmed">
              Стратегия сброса: {settings.trafficLimitStrategy}
            </Text>
          </Stack>
        )}
        {view === "devices" && (
          <Stack>
            {subscription.devicesLoaded ? (
              subscription.devices?.length ? (
                subscription.devices.map((d, i) => (
                  <Card key={d.Hwid || i} shadow="none">
                    <Group gap="sm">
                      <TbDevices2 />
                      <Text size="sm" fw={600}>
                        {d.DeviceModel || d.Platform || "Устройство"}
                      </Text>
                    </Group>
                    <Text size="xs" c="dimmed" mt="xs">
                      {d.Platform} {d.OSVersion}
                    </Text>
                    <Group justify="space-between" mt="xs">
                      <Text size="xs" className="rn-number">
                        {d.Hwid}
                      </Text>
                      <Copy value={d.Hwid} />
                    </Group>
                  </Card>
                ))
              ) : (
                <Text c="dimmed">Устройства не подключены</Text>
              )
            ) : (
              <Text c="dimmed">
                Список HWID не загрузился. Обновите пользователя.
              </Text>
            )}
          </Stack>
        )}
        {view === "access" && (
          <Stack>
            <Text size="sm" c="dimmed">
              Активные внутренние сквады
            </Text>
            {(settings.activeInternalSquads || []).map((id) => (
              <Badge key={id} color="gray" variant="light">
                {squads.internal.find((s) => s.uuid === id)?.name || id}
              </Badge>
            ))}
            {!settings.activeInternalSquads?.length && (
              <Text size="sm">Не выбраны</Text>
            )}
            <Text size="sm" c="dimmed">
              Внешний сквад
            </Text>
            <Text size="sm">
              {squads.external.find(
                (s) => s.uuid === settings.externalSquadUuid,
              )?.name || "Не выбран"}
            </Text>
          </Stack>
        )}
      </Modal>
    </>
  );
}
function Settings({ model, subscription, content, Legacy, dispatch }) {
  const [draft, setDraft] = useState(() =>
    structuredClone(subscription.settings || {}),
  );
  const [error, setError] = useState(""),
    [saving, setSaving] = useState(false),
    [dirty, setDirty] = useState(false),
    [search, setSearch] = useState(""),
    [operations, setOperations] = useState(false);
  useEffect(() => {
    if (!dirty && !saving && subscription.settings) {
      setDraft(structuredClone(subscription.settings));
    }
  }, [subscription.settings, dirty, saving]);
  const disabled = !model.canEditUser || saving || model.userBusy;
  const update = (key, value) => {
    setDraft((d) => ({ ...d, [key]: value }));
    setDirty(true);
  };
  const save = async () => {
    setError("");
    setSaving(true);
    try {
      await model.saveUserSettings(subscription.id, draft);
      setDirty(false);
    } catch (e) {
      setError(e.message || "Не удалось сохранить");
    } finally {
      setSaving(false);
    }
  };
  if (!subscription.settings)
    return (
      <Alert color="yellow">
        Настройки панели не загрузились. Обновите карточку пользователя.
      </Alert>
    );
  const internal = (model.squads.internal || []).filter((s) =>
    s.name.toLocaleLowerCase().includes(search.toLocaleLowerCase().trim()),
  );
  const internalIDs = draft.activeInternalSquads || [];
  const changeSquad = (id, checked) =>
    update(
      "activeInternalSquads",
      checked ? [...internalIDs, id] : internalIDs.filter((v) => v !== id),
    );
  return (
    <>
      <div className="rn-user-columns">
        <Stack gap="md" className="rn-user-left">
          <Identification
            user={model.user}
            subscription={subscription}
            settings={draft}
            squads={model.squads}
          />
          <Section
            className="rn-user-contacts"
            title="Контактные данные"
            icon={TbMail}
            color="teal"
          >
            <Stack gap="md">
              <NumberInput
                label="Telegram ID"
                placeholder="Не указан"
                value={draft.telegramId ?? ""}
                hideControls
                allowNegative={false}
                allowDecimal={false}
                disabled={disabled}
                onChange={(v) =>
                  update("telegramId", v === "" ? null : Number(v))
                }
              />
              <TextInput
                label="Email"
                type="email"
                placeholder="Не указан"
                value={draft.email || ""}
                leftSection={<TbMail size={16} />}
                onChange={(e) => update("email", e.target.value)}
                disabled={disabled}
              />
            </Stack>
          </Section>
          <Section
            className="rn-user-devices"
            title="Устройства и теги"
            icon={TbSettings}
            color="orange"
          >
            <Stack gap="md">
              <Stack gap={0}>
                <Input.Label>Лимит устройств HWID</Input.Label>
                <Input.Description>
                  Индивидуальный лимит устройств. Если поле пустое, используется
                  лимит из настроек панели.
                </Input.Description>
                <Checkbox
                  label="Отключить лимит HWID"
                  my="xs"
                  checked={draft.hwidDeviceLimit === 0}
                  onChange={(e) =>
                    update("hwidDeviceLimit", e.target.checked ? 0 : null)
                  }
                  disabled={disabled}
                />
                <NumberInput
                  placeholder="Лимит из настроек панели"
                  value={draft.hwidDeviceLimit ?? ""}
                  leftSection={<TbDevices2 size={16} />}
                  min={0}
                  max={1000}
                  hideControls
                  allowDecimal={false}
                  disabled={disabled || draft.hwidDeviceLimit === 0}
                  onChange={(v) =>
                    update("hwidDeviceLimit", v === "" ? null : Number(v))
                  }
                />
              </Stack>
              <TagInput
                value={draft.tag || ""}
                onChange={(value) => update("tag", value)}
                disabled={disabled}
                tags={(model.user?.subscriptions || [])
                  .map((item) => item.settings?.tag)
                  .filter(Boolean)}
              />
              <Textarea
                label="Описание"
                description="Краткая заметка о пользователе"
                maxLength={2000}
                autosize
                minRows={2}
                value={draft.description || ""}
                onChange={(e) => update("description", e.target.value)}
                disabled={disabled}
              />
            </Stack>
          </Section>
        </Stack>
        <Stack gap="md" className="rn-user-right">
          <Section
            className="rn-user-traffic"
            title="Трафик и лимиты"
            icon={TbChartLine}
            color="violet"
          >
            <Stack gap="md">
              <TrafficInput
                value={draft.trafficLimitBytes}
                onChange={(v) => update("trafficLimitBytes", v)}
                disabled={disabled}
              />
              <Select
                label="Стратегия сброса трафика"
                description="Как часто обнулять статистику трафика пользователя"
                leftSection={<PiClockDuotone size={16} />}
                allowDeselect={false}
                data={[
                  { value: "NO_RESET", label: "Никогда" },
                  { value: "DAY", label: "Ежедневно" },
                  { value: "WEEK", label: "Еженедельно" },
                  { value: "MONTH", label: "Ежемесячно" },
                ]}
                value={draft.trafficLimitStrategy}
                onChange={(v) => update("trafficLimitStrategy", v)}
                comboboxProps={{ withinPortal: false }}
                disabled={disabled}
              />
            </Stack>
          </Section>
          <Section
            className="rn-user-access"
            title="Настройки доступа"
            icon={TbShield}
            color="indigo"
          >
            <Stack gap="md">
              <DateTimePicker
                label="Дата окончания"
                description="Дата и время, до которых пользователь имеет доступ"
                dropdownType="modal"
                value={
                  draft.expireAt
                    ? dayjs(draft.expireAt).format("YYYY-MM-DD HH:mm:ss")
                    : null
                }
                onChange={(v) => {
                  if (v && dayjs(v).isValid())
                    update("expireAt", dayjs(v).toISOString());
                }}
                valueFormat="MMMM D, YYYY - HH:mm"
                locale={model.locale === "en" ? "en" : "ru"}
                headerControlsOrder={["previous", "next", "level"]}
                highlightToday
                leftSection={<PiCalendarDuotone size={16} />}
                disabled={disabled}
                modalProps={{
                  centered: true,
                  withinPortal: false,
                  zIndex: 330,
                  title: "Дата окончания",
                }}
                submitButtonProps={{
                  style: {
                    borderRadius: "var(--mantine-radius-md)",
                    width: "20%",
                  },
                }}
                timePickerProps={{
                  withDropdown: true,
                  presets: getTimeRange({
                    startTime: "06:00:00",
                    endTime: "18:00:00",
                    interval: "01:30:00",
                  }),
                }}
                presets={[
                  ...[1, 3, 12].map((n) => ({
                    value: dayjs(draft.expireAt || undefined)
                      .add(n, "month")
                      .format("YYYY-MM-DD HH:mm:ss"),
                    label: n === 12 ? "+1 год" : `+${n} мес.`,
                  })),
                  {
                    value: dayjs().year(2099).format("YYYY-MM-DD HH:mm:ss"),
                    label: "2099 год",
                  },
                ]}
              />
              <Stack gap={6}>
                <Input.Label>Внутренние сквады</Input.Label>
                <Input.Description>
                  Укажите, в каких внутренних сквадах будет состоять
                  пользователь.
                </Input.Description>
                <TextInput
                  placeholder="Поиск сквадов"
                  aria-label="Поиск внутренних сквадов"
                  leftSection={<TbSearch size={16} />}
                  value={search}
                  onChange={(e) => setSearch(e.target.value)}
                />
                <Stack gap="xs" className="rn-squad-list">
                  {internal.map((s) => (
                    <Checkbox.Card
                      key={s.uuid}
                      checked={internalIDs.includes(s.uuid)}
                      onClick={() =>
                        !disabled &&
                        changeSquad(s.uuid, !internalIDs.includes(s.uuid))
                      }
                      disabled={disabled}
                      p="sm"
                      radius="md"
                    >
                      <Group justify="space-between" wrap="nowrap">
                        <Group gap="xs" wrap="nowrap">
                          <Checkbox.Indicator />
                          <Text size="sm" fw={500}>
                            {s.name}
                          </Text>
                        </Group>
                        {s.usersCount != null && (
                          <Badge variant="light" color="gray">
                            {s.usersCount}
                          </Badge>
                        )}
                      </Group>
                    </Checkbox.Card>
                  ))}
                  {!internal.length && (
                    <Text c="dimmed" size="xs" py="xs">
                      {search ? "Сквады не найдены" : "Внутренних сквадов нет"}
                    </Text>
                  )}
                </Stack>
              </Stack>
              <Select
                label="Внешний сквад"
                description="Индивидуальные настройки подписки для этого пользователя"
                clearable
                searchable
                placeholder="Выберите внешний сквад"
                leftSection={<TbWebhook size={16} />}
                data={(model.squads.external || []).map((s) => ({
                  value: s.uuid,
                  label: s.name,
                }))}
                value={draft.externalSquadUuid || null}
                onChange={(v) => update("externalSquadUuid", v)}
                comboboxProps={{ withinPortal: false }}
                disabled={disabled}
              />
            </Stack>
          </Section>
        </Stack>
      </div>
      {error && (
        <Alert color="red" mt="md">
          {error}
        </Alert>
      )}
      {operations && (
        <Accordion mt="md" variant="separated" defaultValue="bot">
          <Accordion.Item value="bot">
            <Accordion.Control>
              Баланс, подписки, сообщения и блокировка
            </Accordion.Control>
            <Accordion.Panel>
              <div className="rn-content rn-user-operations">
                <Legacy node={content} />
              </div>
            </Accordion.Panel>
          </Accordion.Item>
        </Accordion>
      )}
      <footer className="rn-modal-footer">
        <Group justify="flex-end" gap="md">
          <Menu position="top-end">
            <Menu.Target>
              <Button
                className="rn-more-actions"
                color="gray"
                size="md"
                leftSection={<TbDots size={20} />}
              >
                Ещё действия
              </Button>
            </Menu.Target>
            <Menu.Dropdown>
              <Menu.Item onClick={() => setOperations(true)}>
                Баланс и действия в боте
              </Menu.Item>
              <Menu.Item
                onClick={() => dispatch("admin-user-reissue-subscription")}
                disabled={disabled}
              >
                Перевыпустить подписку
              </Menu.Item>
              <Menu.Divider />
              <Menu.Item color="red" onClick={() => setOperations(true)}>
                Блокировка и удаление подписки
              </Menu.Item>
            </Menu.Dropdown>
          </Menu>
          <Button
            color="teal"
            size="md"
            leftSection={<PiFloppyDiskDuotone size={16} />}
            disabled={!dirty || disabled}
            loading={saving}
            onClick={save}
          >
            Сохранить
          </Button>
        </Group>
      </footer>
    </>
  );
}
export function UserDialog({ model, content, dispatch, Legacy }) {
  const user = model.user;
  const subscription =
    user?.subscriptions?.find(
      (s) => String(s.id) === String(model.selectedSubscriptionID),
    ) || user?.subscriptions?.[0];
  return (
    <Modal
      opened
      onClose={() => dispatch("admin-user-back")}
      title={
        <Group gap="sm">
          <ThemeIcon variant="soft" color="cyan" size={34}>
            <TbUser size={20} />
          </ThemeIcon>
          <Text fw={600}>
            {subscription?.panelUsername || user?.username || "Пользователь"}
          </Text>
        </Group>
      }
      size={1000}
      padding="sm"
      className="rn-user-modal"
      closeButtonProps={{ "aria-label": "Закрыть пользователя" }}
      overlayProps={{ backgroundOpacity: 0.65 }}
    >
      {user?.subscriptions?.length > 1 && (
        <Group gap="xs" mb="md">
          {user.subscriptions.map((s) => (
            <Button
              key={s.id}
              data-action="admin-user-view-subscription"
              data-value={s.id}
              variant={s.id === subscription?.id ? "filled" : "light"}
            >
              {s.name || `Подписка ${s.id}`}
            </Button>
          ))}
        </Group>
      )}
      {subscription ? (
        <Settings
          key={`${user.customerId}:${subscription.id}`}
          model={model}
          subscription={subscription}
          content={content}
          dispatch={dispatch}
          Legacy={Legacy}
        />
      ) : (
        <Text c="dimmed" p="md">
          {user ? "Нет подписок" : "Загрузка пользователя…"}
        </Text>
      )}
    </Modal>
  );
}

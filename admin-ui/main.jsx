import React, { useState, useRef, useLayoutEffect, useMemo } from "react";
import "@fontsource/fira-mono/400.css";
import "@fontsource/fira-mono/500.css";
import "@fontsource/fira-mono/700.css";
import "@fontsource/unbounded/700.css";
import { createRoot } from "react-dom/client";
import { flushSync } from "react-dom";
import {
  MantineProvider,
  createTheme,
  defaultVariantColorsResolver,
  Button,
  ActionIcon,
  Card,
  Group,
  Stack,
  Text,
  Title,
  ThemeIcon,
  Menu,
  Modal,
  Drawer,
  NavLink,
  Divider,
  Badge,
  TextInput,
  Textarea,
  NativeSelect,
  Switch,
  Checkbox,
  Tooltip,
  SimpleGrid,
  NumberInput,
  MultiSelect,
  Select,
  Progress,
  Accordion,
  Alert,
  UnstyledButton,
  Tabs,
  CopyButton,
} from "@mantine/core";
import {
  MantineReactTable,
  useMantineReactTable,
} from "@kastov/mantine-react-table-open";
import { MRT_Localization_RU } from "@kastov/mantine-react-table-open/locales/ru/index.esm.mjs";
import {
  PiUsers,
  PiStar,
  PiSlidersHorizontal,
  PiCaretDown,
  PiSignOut,
  PiArrowsClockwise,
  PiPlus,
  PiPencilSimple,
  PiTrash,
  PiList,
  PiSquaresFour,
  PiShieldCheck,
  PiCreditCard,
  PiGlobe,
  PiArrowSquareOut,
  PiCheck,
  PiHardDrives,
  PiX,
  PiGear,
  PiChartLine,
  PiDevices,
  PiLightning,
  PiEye,
  PiEyeSlash,
} from "react-icons/pi";
import "@mantine/core/styles.css";
import "@mantine/dates/styles.css";
import "@kastov/mantine-react-table-open/styles.css";
import "./styles.css";
import "./reference-theme.css";
import { UserDialog } from "./user-dialog.jsx";
import { LayoutEditor as Layout } from "./layout-editor.jsx";
import {
  TbBrandGithub,
  TbLanguage,
  TbBook,
  TbChartBar,
  TbLayersIntersect,
  TbMessageCircle,
  TbDiscount,
  TbMail,
  TbPlug,
  TbFileText,
  TbPalette,
  TbActivity,
  TbTestPipe,
  TbToggleRight,
  TbClock,
  TbAlertTriangle,
  TbSparkles,
  TbBell,
  TbCopy,
} from "react-icons/tb";
import {
  PiClockCountdownDuotone,
  PiClockUserDuotone,
  PiProhibitDuotone,
  PiPulseDuotone,
  PiUsersDuotone,
} from "react-icons/pi";

// Theme and navigation proportions adapted from remnawave/frontend, AGPL-3.0.
const theme = createTheme({
  fontFamily: "Montserrat, sans-serif",
  fontFamilyMonospace: '"Fira Mono", "JetBrains Mono", monospace',
  primaryColor: "cyan",
  primaryShade: 8,
  autoContrast: true,
  luminanceThreshold: 0.3,
  breakpoints: { xs: "30em", sm: "40em", md: "48em", lg: "64em", xl: "80em" },
  defaultRadius: "md",
  radius: { md: "6px", lg: "10px" },
  headings: { fontWeight: "600" },
  focusRing: "auto",
  colors: {
    dark: [
      "#e8ecf2",
      "#ccd2db",
      "#a8b0bc",
      "#7e8896",
      "#262628",
      "#1b1c1e",
      "#101113",
      "#0b0c0e",
      "#050607",
      "#040506",
    ],
  },
  variantColorResolver(input) {
    if (input.variant === "soft") {
      const c = input.theme.colors[input.color || "gray"];
      return {
        background: `linear-gradient(135deg, ${c[6]}26, ${c[7]}1a)`,
        border: `1px solid ${c[6]}4d`,
        color: `var(--mantine-color-${input.color || "gray"}-4)`,
        hover: c[4] + "1a",
      };
    }
    return defaultVariantColorsResolver(input);
  },
  components: {
    Button: Button.extend({
      defaultProps: { variant: "light", radius: "md" },
      styles: { root: { transition: "all .2s ease" } },
    }),
    ActionIcon: ActionIcon.extend({
      defaultProps: { variant: "outline", radius: "md" },
    }),
    Card: Card.extend({
      defaultProps: { withBorder: true, padding: "md", shadow: "xl" },
    }),
    Modal: Modal.extend({
      defaultProps: {
        centered: true,
        radius: "md",
        withinPortal: false,
        zIndex: 300,
        transitionProps: { transition: "fade", duration: 200 },
      },
    }),
    Menu: Menu.extend({
      defaultProps: { withinPortal: false, shadow: "md", radius: "md" },
    }),
    Tooltip: Tooltip.extend({
      defaultProps: {
        radius: "md",
        withArrow: true,
        transitionProps: { transition: "scale-x", duration: 300 },
        arrowSize: 2,
        color: "dark.6",
        styles: {
          tooltip: { border: "1px solid var(--mantine-color-dark-4)" },
        },
      },
    }),
  },
});
let mountedRoot, mountedElement;
const glyphs = {
  users: PiUsers,
  cartShopping: PiCreditCard,
  server: PiHardDrives,
  chartLine: PiChartLine,
  grid: PiSquaresFour,
  adminSubscriptions: TbLayersIntersect,
  sms: TbMessageCircle,
  star: PiStar,
  adminPromocodes: TbDiscount,
  adminBroadcast: TbMail,
  adminIntegrations: TbPlug,
  adminContent: TbFileText,
  adminAppearance: TbPalette,
  language: TbLanguage,
  adminDiagnostics: TbActivity,
  adminFeatures: TbToggleRight,
  adminTrial: TbClock,
  adminMaintenance: TbAlertTriangle,
  sparkles: TbSparkles,
  adminPush: TbBell,
};
const glyph = (name) => {
  const Icon = glyphs[name] || PiSlidersHorizontal;
  return <Icon size={18} />;
};
const actionProps = (action, value, extra = {}) => ({
  "data-action": action,
  "data-value": value,
  ...extra,
});

// Keep the authenticated application's event delegation and draft validation intact.
// Native inputs remain uncontrolled: typing does not replace a field or lose its caret.
function Field({ node, label, toggle = false }) {
  const p = attributes(node),
    ref = useRef(null);
  const isCheck = node.type === "checkbox" || node.type === "radio";
  const [checked, setChecked] = useState(node.checked);
  useLayoutEffect(() => {
    if (isCheck) setChecked(node.checked);
  }, [node.checked]);
  useLayoutEffect(() => {
    if (ref.current && document.activeElement !== ref.current) {
      if (isCheck) ref.current.checked = node.checked;
      else if (node.type !== "file") ref.current.value = node.value;
    }
  }, [node.value, node.checked]);
  delete p.className;
  delete p.style;
  delete p.value;
  delete p.checked;
  const common = {
    ...p,
    ref,
    label,
    defaultValue: isCheck || node.type === "file" ? undefined : node.value,
  };
  if (isCheck) {
    const Control = toggle ? Switch : Checkbox;
    return (
      <Control
        {...common}
        checked={checked}
        onChange={(e) => setChecked(e.currentTarget.checked)}
        label={label}
      />
    );
  }
  if (node.tagName === "SELECT")
    return (
      <NativeSelect
        {...common}
        data={[...node.options].map((o) => ({
          value: o.value,
          label: o.textContent,
          disabled: o.disabled,
        }))}
      />
    );
  if (node.tagName === "TEXTAREA")
    return <Textarea {...common} autosize minRows={Number(node.rows) || 3} />;
  return <TextInput {...common} />;
}
function attributes(node) {
  const p = {};
  for (const a of node.attributes) {
    const n = a.name;
    if (n === "style") {
      const style = {};
      for (const k of node.style)
        style[
          k.startsWith("--")
            ? k
            : k.replace(/-([a-z])/g, (_, c) => c.toUpperCase())
        ] = node.style.getPropertyValue(k);
      p.style = style;
    } else if (n === "class") p.className = a.value;
    else if (n === "readonly") p.readOnly = true;
    else if (n === "for") p.htmlFor = a.value;
    else if (
      [
        "disabled",
        "hidden",
        "multiple",
        "required",
        "readOnly",
        "checked",
      ].includes(n)
    )
      p[n] = true;
    else if (n === "value") p.defaultValue = a.value;
    else if (n === "tabindex") p.tabIndex = Number(a.value);
    else if (n === "maxlength") p.maxLength = Number(a.value);
    else if (n === "inputmode") p.inputMode = a.value;
    else if (!n.startsWith("on") && n !== "selected") p[n] = a.value;
  }
  return p;
}
function convert(node, key) {
  if (node.nodeType === 3) return node.textContent;
  if (node.nodeType !== 1) return null;
  const tag = node.tagName.toLowerCase(),
    p = attributes(node);
  p.key = key;
  if (tag === "script" || tag === "style") return null;
  if (
    node.matches(
      ".admin-editor__header,.admin-save-bar,.rw-savebar,.admin-editor__save,.admin-plan-editor",
    )
  )
    return null;
  if (tag === "svg")
    return <svg {...p} dangerouslySetInnerHTML={{ __html: node.innerHTML }} />;
  if (node.dataset.appIcon && glyphs[node.dataset.appIcon]) {
    const Icon = glyphs[node.dataset.appIcon];
    return <Icon key={key} size={20} aria-hidden="true" />;
  }
  if (
    node.matches(".tabs") &&
    node.querySelector(":scope > button[data-action]")
  ) {
    const tabs = [...node.querySelectorAll(":scope > button[data-action]")];
    const selected =
      tabs.find((tab) => tab.classList.contains("active")) || tabs[0];
    return (
      <Tabs key={key} value={selected.dataset.value} className="rn-native-tabs">
        <Tabs.List>
          {tabs.map((tab) => (
            <Tabs.Tab
              key={tab.dataset.value}
              value={tab.dataset.value}
              data-action={tab.dataset.action}
              data-value={tab.dataset.value}
              disabled={tab.disabled}
            >
              {tab.textContent}
            </Tabs.Tab>
          ))}
        </Tabs.List>
      </Tabs>
    );
  }
  if (tag === "label") {
    const control = node.querySelector(
      ":scope > input,:scope > textarea,:scope > select",
    );
    if (control) {
      if (control.type === "file") {
        const fileProps = attributes(control);
        delete fileProps.defaultValue;
        const Control = node.classList.contains("support-reply__attach")
          ? ActionIcon
          : Button;
        const label =
          node.getAttribute("aria-label") ||
          node.textContent.trim() ||
          "Выбрать файл";
        return (
          <Control
            key={key}
            component="label"
            variant="outline"
            size="lg"
            className="rn-file-picker"
            aria-label={label}
            disabled={control.disabled}
            role="button"
            tabIndex={control.disabled ? -1 : 0}
            onKeyDown={(event) => {
              if (!control.disabled && ["Enter", " "].includes(event.key)) {
                event.preventDefault();
                event.currentTarget.querySelector("input").click();
              }
            }}
          >
            {[...node.childNodes]
              .filter((n) => n !== control)
              .map((n, i) => convert(n, i))}
            <input {...fileProps} className="rn-file-input" />
          </Control>
        );
      }
      const toggle = node.classList.contains("admin-toggle");
      const label = [...node.childNodes]
        .filter(
          (n) =>
            n !== control &&
            !(n.nodeType === 1 && n.getAttribute("aria-hidden") === "true"),
        )
        .map((n, i) => convert(n, i));
      return (
        <div
          key={key}
          className={`rn-field ${toggle ? "rn-toggle" : node.className}`}
          hidden={node.hidden}
        >
          <Field node={control} label={label} toggle={toggle} />
        </div>
      );
    }
  }
  if (["input", "textarea", "select"].includes(tag))
    return <Field key={key} node={node} />;
  const children = [...node.childNodes].map((n, i) => convert(n, i));
  if (tag === "button") {
    if (
      node.matches(".card,.menu-card,.menu-row,.profile-row") ||
      node.querySelector("div,strong,small,img,video,b,article,section")
    ) {
      p.className = `${p.className || ""} rn-structured-button ${node.matches(".card,.menu-card,.menu-row,.profile-row") ? "rn-clickable-card" : ""}`;
      return <UnstyledButton {...p}>{children}</UnstyledButton>;
    }
    const danger = /delete|remove|block$|reject/.test(
      node.dataset.action || "",
    );
    const name = node.getAttribute("aria-label") || node.textContent.trim();
    p.className = `${p.className || ""} rn-adapted-button`;
    if (!node.textContent.trim())
      return (
        <ActionIcon
          {...p}
          aria-label={name || "Открыть"}
          color={danger ? "red" : "gray"}
          size="lg"
        >
          {children}
        </ActionIcon>
      );
    return (
      <Button {...p} color={danger ? "red" : "gray"} size="sm">
        {children}
      </Button>
    );
  }
  return [
    "img",
    "br",
    "hr",
    "area",
    "base",
    "col",
    "embed",
    "link",
    "meta",
    "param",
    "source",
    "track",
    "wbr",
  ].includes(tag)
    ? React.createElement(tag, p)
    : React.createElement(tag, p, children);
}
function Legacy({ node }) {
  return node ? convert(node, "content") : null;
}
function Action({ action, value, label, children, ...props }) {
  return (
    <Button {...actionProps(action, value)} {...props}>
      {children || label}
    </Button>
  );
}
function IconAction({ action, value, label, children, ...props }) {
  return (
    <Tooltip label={label} withinPortal={false}>
      <ActionIcon
        aria-label={label}
        {...actionProps(action, value)}
        size="lg"
        {...props}
      >
        {children}
      </ActionIcon>
    </Tooltip>
  );
}
function Metric({
  title,
  value,
  icon: Icon = PiUsers,
  color = "gray",
  caption,
}) {
  return (
    <Card className="rn-metric">
      <Group wrap="nowrap" gap="md">
        <ThemeIcon size="xl" radius="lg" variant="soft" color={color}>
          <Icon size={24} />
        </ThemeIcon>
        <Stack gap={0}>
          <Text size="sm" c="dimmed">
            {title}
          </Text>
          <Text className="rn-number" size="xl" fw={700}>
            {value}
          </Text>
          {caption && (
            <Text size="xs" c="dimmed">
              {caption}
            </Text>
          )}
        </Stack>
      </Group>
    </Card>
  );
}
function DataTable({ columns, data, id, extra }) {
  const initial = useMemo(() => {
    try {
      return JSON.parse(localStorage.getItem("admin.table." + id)) || {};
    } catch {
      return {};
    }
  }, [id]);
  const [density, setDensity] = useState(initial.density || "xxs");
  const [columnVisibility, setVisibility] = useState(
    initial.columnVisibility || {},
  );
  const [columnOrder, setOrder] = useState(initial.columnOrder || []);
  useLayoutEffect(() => {
    try {
      localStorage.setItem(
        "admin.table." + id,
        JSON.stringify({ density, columnVisibility, columnOrder }),
      );
    } catch {}
  }, [density, columnVisibility, columnOrder, id]);
  const table = useMantineReactTable({
    columns,
    data,
    localization: MRT_Localization_RU,
    enableColumnOrdering: true,
    enableColumnResizing: true,
    enableColumnPinning: true,
    enableRowSelection: true,
    enableStickyHeader: true,
    enableFilters: true,
    layoutMode: "grid",
    displayColumnDefOptions: { "mrt-row-select": { size: 40, grow: false } },
    initialState: {
      showColumnFilters: true,
      pagination: { pageSize: 20, pageIndex: 0 },
      columnPinning: { right: ["actions"] },
    },
    state: {
      density,
      columnVisibility,
      ...(columnOrder.length ? { columnOrder } : {}),
    },
    onDensityChange: setDensity,
    onColumnVisibilityChange: setVisibility,
    onColumnOrderChange: setOrder,
    mantinePaperProps: { withBorder: false, radius: 0, shadow: "none" },
    mantineTableProps: { highlightOnHover: true },
    mantineFilterTextInputProps: { variant: "unstyled", placeholder: "Filter" },
    mantineTableContainerProps: {
      style: { maxHeight: "calc(100dvh - 350px)" },
    },
    mantineTableHeadCellProps: {
      style: { fontSize: 13, background: "#101113" },
    },
    mantineTopToolbarProps: { style: { background: "#101113" } },
    mantineBottomToolbarProps: { style: { background: "#101113" } },
    mantineTableBodyCellProps: ({ column }) => ({
      style: {
        fontSize: 13,
        background: column.getIsPinned() ? "#101113" : undefined,
      },
    }),
    renderTopToolbarCustomActions: () => extra,
  });
  return <MantineReactTable table={table} />;
}
function UserMetrics({ users }) {
  const counts = users.panelCounts;
  return (
    <SimpleGrid
      cols={{ base: 1, xs: 2, xl: 5 }}
      spacing="xs"
      mb="md"
      title="Статистика пользователей Remnawave"
    >
      {[
        ["Всего", counts?.totalUsers, PiUsersDuotone, "blue"],
        ["Active", counts?.statusCounts?.ACTIVE, PiPulseDuotone, "teal"],
        ["Expired", counts?.statusCounts?.EXPIRED, PiClockUserDuotone, "red"],
        [
          "Limited",
          counts?.statusCounts?.LIMITED,
          PiClockCountdownDuotone,
          "orange",
        ],
        ["Disabled", counts?.statusCounts?.DISABLED, PiProhibitDuotone, "gray"],
      ].map(([title, value, Icon, color]) => (
        <Metric
          key={title}
          title={title}
          value={value == null ? "—" : Number(value).toLocaleString("ru")}
          icon={Icon}
          color={color}
        />
      ))}
    </SimpleGrid>
  );
}
function Plans({ model }) {
  const columns = useMemo(
    () => [
      {
        accessorKey: "name",
        header: "Название",
        Cell: ({ row }) => (
          <Action
            action="admin-edit-plan"
            value={row.original.id}
            variant="subtle"
          >
            {row.original.name ||
              `${row.original.days || row.original.months} ${row.original.days ? "дней" : "мес."}`}
          </Action>
        ),
      },
      {
        accessorKey: "enabled",
        header: "Статус",
        Cell: ({ cell, row }) => (
          <Switch
            aria-label="Тариф активен"
            key={String(cell.getValue())}
            defaultChecked={Boolean(cell.getValue())}
            data-setting-path={`plans.${row.index}.enabled`}
            data-setting-type="boolean"
            label={cell.getValue() ? "ACTIVE" : "DISABLED"}
          />
        ),
      },
      {
        id: "duration",
        header: "Срок",
        accessorFn: (p) => (p.days ? `${p.days} дней` : `${p.months} мес.`),
      },
      { accessorKey: "priceRub", header: "Цена, ₽" },
      {
        accessorKey: "trafficGb",
        header: "Трафик, ГБ",
        Cell: ({ cell }) => cell.getValue() || "∞",
      },
      {
        accessorKey: "deviceLimit",
        header: "Устройства",
        Cell: ({ cell }) => cell.getValue() || "∞",
      },
      {
        id: "actions",
        header: "Действия",
        enableColumnFilter: false,
        enableSorting: false,
        Cell: ({ row }) => (
          <Group gap={6}>
            <IconAction
              action="admin-edit-plan"
              value={row.original.id}
              label="Редактировать"
            >
              <PiPencilSimple />
            </IconAction>
            <IconAction
              action="admin-toggle-plan-wide"
              value={row.original.id}
              label="Широкая карточка тарифа"
            >
              <PiSquaresFour />
            </IconAction>
            <IconAction
              action="admin-delete-plan"
              value={row.original.id}
              label="Удалить тариф"
              color="red"
            >
              <PiTrash />
            </IconAction>
          </Group>
        ),
      },
    ],
    [],
  );
  return (
    <Stack gap="md">
      <Card p={0} className="rn-table-card">
        <DataTable
          columns={columns}
          data={model.plans}
          id="plans"
          extra={
            <Group gap="xs" p="xs">
              <Action
                action="admin-open-device-packs"
                leftSection={<PiDevices />}
              >
                Пакеты устройств
              </Action>
              <Action
                action="admin-open-traffic-packs"
                leftSection={<PiChartLine />}
              >
                Пакеты трафика
              </Action>
            </Group>
          }
        />
      </Card>
      <Accordion variant="separated">
        <Accordion.Item value="order">
          <Accordion.Control>
            Порядок тарифов и способов оплаты
          </Accordion.Control>
          <Accordion.Panel>
            <SimpleGrid cols={{ base: 1, sm: 2 }}>
              <Stack gap="xs">
                {model.plans.map((p, i) => (
                  <Group key={p.id} justify="space-between">
                    <Text size="sm">{p.name || p.titleRu || p.id}</Text>
                    <Group gap={4}>
                      <Action
                        action="admin-move-plan"
                        value={i}
                        data-direction="-1"
                        disabled={i === 0}
                      >
                        ↑
                      </Action>
                      <Action
                        action="admin-move-plan"
                        value={i}
                        data-direction="1"
                        disabled={i === model.plans.length - 1}
                      >
                        ↓
                      </Action>
                    </Group>
                  </Group>
                ))}
              </Stack>
              <Stack gap="xs">
                {model.paymentMethods.map((p, i) => (
                  <Group
                    key={p.id}
                    justify="space-between"
                    data-pay-order-id={p.id}
                  >
                    <Text size="sm">{p.label}</Text>
                    <Group gap={4}>
                      <Action
                        action="move-pay-method-up"
                        value={p.id}
                        disabled={i === 0}
                      >
                        ↑
                      </Action>
                      <Action
                        action="move-pay-method-down"
                        value={p.id}
                        disabled={i === model.paymentMethods.length - 1}
                      >
                        ↓
                      </Action>
                    </Group>
                  </Group>
                ))}
              </Stack>
            </SimpleGrid>
          </Accordion.Panel>
        </Accordion.Item>
      </Accordion>
    </Stack>
  );
}
function SubscriptionLink({ value }) {
  if (!value) return null;
  return (
    <Group gap={6} wrap="nowrap" style={{ width: "100%", minWidth: 0 }}>
      <Tooltip label={value}>
        <Text size="xs" c="dimmed" truncate style={{ minWidth: 0, flex: 1 }}>
          {value}
        </Text>
      </Tooltip>
      <CopyButton value={value}>
        {({ copied, copy }) => (
          <Tooltip label={copied ? "Скопировано" : "Копировать подписку"}>
            <ActionIcon
              onClick={copy}
              aria-label="Копировать подписку"
              size="sm"
              color={copied ? "teal" : "gray"}
            >
              <TbCopy size={16} />
            </ActionIcon>
          </Tooltip>
        )}
      </CopyButton>
    </Group>
  );
}
function Users({ model }) {
  const columns = useMemo(
    () => [
      {
        id: "username",
        header: "Юзернейм",
        size: 130,
        accessorFn: (u) => u.username || u.firstName || String(u.telegramId),
        Cell: ({ row, cell }) => (
          <Action
            action="admin-user-open"
            value={row.original.customerId}
            variant="subtle"
          >
            {cell.getValue()}
          </Action>
        ),
      },
      { accessorKey: "customerId", header: "ID", size: 60 },
      { accessorKey: "telegramId", header: "Telegram ID", size: 130 },
      {
        id: "status",
        header: "Статус",
        size: 110,
        accessorFn: (u) =>
          u.isBlocked ? "blocked" : u.subscriptionStatus || "none",
        Cell: ({ cell }) => (
          <Badge
            radius="sm"
            variant="light"
            color={
              cell.getValue() === "active"
                ? "teal"
                : cell.getValue() === "blocked"
                  ? "red"
                  : "gray"
            }
          >
            {String(cell.getValue()).toUpperCase()}
          </Badge>
        ),
      },
      {
        accessorKey: "subscriptionName",
        header: "Подписка",
        size: 190,
        Cell: ({ row, cell }) => (
          <Stack gap={2} style={{ width: "100%", minWidth: 0 }}>
            <Text size="sm" truncate>
              {cell.getValue() || "Без подписки"}
            </Text>
            <SubscriptionLink value={row.original.subscriptionLink} />
            {row.original.panelUsername && (
              <Text size="xs" c="dimmed" truncate>
                {row.original.panelUsername}
              </Text>
            )}
            {row.original.subscriptionCount > 1 && (
              <Text size="xs" c="dimmed">
                Всего подписок: {row.original.subscriptionCount}
              </Text>
            )}
          </Stack>
        ),
      },
      {
        id: "traffic",
        header: "Трафик",
        size: 155,
        accessorFn: (user) =>
          user.trafficLoaded ? user.usedTrafficBytes : null,
        Cell: ({ row }) => {
          const user = row.original;
          if (!user.trafficLoaded)
            return (
              <Text size="xs" c="dimmed">
                Не загружен
              </Text>
            );
          const format = (bytes) => {
            if (!bytes) return "0 B";
            const unit = Math.min(
              4,
              Math.floor(Math.log(bytes) / Math.log(1024)),
            );
            return `${(bytes / 1024 ** unit).toFixed(unit ? 2 : 0)} ${["B", "KiB", "MiB", "GiB", "TiB"][unit]}`;
          };
          const ratio =
            user.trafficLimitBytes > 0
              ? Math.min(
                  100,
                  (user.usedTrafficBytes / user.trafficLimitBytes) * 100,
                )
              : 0;
          return (
            <Stack gap={5}>
              <Text size="xs" className="rn-number">
                {format(user.usedTrafficBytes)} /{" "}
                {user.trafficLimitBytes ? format(user.trafficLimitBytes) : "∞"}
              </Text>
              {user.trafficLimitBytes > 0 && (
                <Progress
                  size={3}
                  value={ratio}
                  color={ratio >= 100 ? "red" : "teal"}
                />
              )}
            </Stack>
          );
        },
      },
      {
        accessorKey: "expiresAt",
        header: "Дата окончания",
        size: 150,
        Cell: ({ cell }) => (
          <Text size="xs">
            {cell.getValue()
              ? new Date(cell.getValue()).toLocaleString("ru", {
                  day: "2-digit",
                  month: "2-digit",
                  year: "numeric",
                  hour: "2-digit",
                  minute: "2-digit",
                })
              : "—"}
          </Text>
        ),
      },
      {
        id: "actions",
        header: "",
        size: 48,
        grow: false,
        enableColumnFilter: false,
        Cell: ({ row }) => (
          <IconAction
            action="admin-user-open"
            value={row.original.customerId}
            label="Открыть пользователя"
          >
            <PiPencilSimple />
          </IconAction>
        ),
      },
    ],
    [],
  );
  return (
    <Card p={0} className="rn-table-card">
      <DataTable
        columns={columns}
        data={model.users.items || []}
        id="users-overview"
        extra={
          <TextInput
            m="xs"
            placeholder="@username, Telegram ID или подписка"
            aria-label="Найти пользователя"
            data-input="admin-users-search"
            defaultValue={model.usersQuery}
            leftSection={<PiUsers />}
          />
        }
      />
      {model.users.items?.length < model.users.total && (
        <Group justify="center" p="md">
          <Text size="xs" c="dimmed">
            Загружено {model.users.items.length} из{" "}
            {Number(model.users.total).toLocaleString("ru")}
          </Text>
          <Action action="admin-users-more">Показать ещё</Action>
        </Group>
      )}
    </Card>
  );
}
function Nodes({ model }) {
  const items = model.servers || [];
  const online = items.filter((item) => item.online).length;
  const columns = useMemo(
    () => [
      {
        accessorKey: "name",
        header: "Название",
        Cell: ({ row }) => (
          <Group gap="xs" wrap="nowrap">
            <Text>
              {/^[a-z]{2}$/i.test(row.original.countryCode || "")
                ? String.fromCodePoint(
                    ...row.original.countryCode
                      .toUpperCase()
                      .split("")
                      .map((c) => c.charCodeAt(0) + 127397),
                  )
                : ""}
            </Text>
            <Text size="sm" fw={500}>
              {row.original.name}
            </Text>
          </Group>
        ),
      },
      {
        id: "status",
        header: "Статус",
        accessorFn: (item) => (item.online ? "ONLINE" : "OFFLINE"),
        Cell: ({ cell }) => (
          <Badge
            radius="sm"
            variant="light"
            color={cell.getValue() === "ONLINE" ? "teal" : "red"}
          >
            {cell.getValue()}
          </Badge>
        ),
      },
      { accessorKey: "address", header: "Адрес" },
      { accessorKey: "countryCode", header: "Страна", size: 100 },
      {
        id: "visibility",
        header: "В кабинете",
        accessorFn: (item) => (item.hidden ? "Скрыта" : "Видна"),
        Cell: ({ row, cell }) => (
          <Badge color="gray" variant="light">
            {cell.getValue()}
          </Badge>
        ),
      },
      ...(model.canManageNodes
        ? [
            {
              id: "actions",
              header: "Действия",
              enableColumnFilter: false,
              Cell: ({ row }) => (
                <IconAction
                  action="admin-toggle-server-visibility"
                  value={row.original.id}
                  label={
                    row.original.hidden
                      ? "Показать ноду пользователям"
                      : "Скрыть ноду от пользователей"
                  }
                  aria-pressed={!row.original.hidden}
                  disabled={!row.original.id || Boolean(model.nodeBusy)}
                >
                  {row.original.hidden ? <PiEyeSlash /> : <PiEye />}
                </IconAction>
              ),
            },
          ]
        : []),
    ],
    [model.canManageNodes, model.nodeBusy],
  );
  const visible = items.filter((item) =>
    model.serverFilter === "online"
      ? item.online
      : model.serverFilter === "offline"
        ? !item.online
        : true,
  );
  return (
    <Stack gap="md">
      <SimpleGrid cols={{ base: 1, sm: 3 }} spacing="xs">
        <Metric
          title="Всего нод"
          value={items.length}
          icon={PiHardDrives}
          color="blue"
        />
        <Metric
          title="Онлайн"
          value={online}
          icon={PiPulseDuotone}
          color="teal"
        />
        <Metric
          title="Офлайн"
          value={items.length - online}
          icon={PiProhibitDuotone}
          color="red"
        />
      </SimpleGrid>
      <Card p={0} className="rn-table-card">
        <DataTable
          id="nodes"
          columns={columns}
          data={visible}
          extra={
            <Group gap="xs" p="xs">
              {[
                ["all", "Все"],
                ["online", "Онлайн"],
                ["offline", "Офлайн"],
              ].map(([id, label]) => (
                <Action
                  key={id}
                  action="set-server-filter"
                  value={id}
                  variant={
                    (model.serverFilter || "all") === id ? "filled" : "light"
                  }
                >
                  {label}
                </Action>
              ))}
            </Group>
          }
        />
      </Card>
    </Stack>
  );
}
function Reviews({ model }) {
  const reviews = model.reviews;
  const columns = useMemo(
    () => [
      {
        accessorKey: "username",
        header: "Пользователь",
        Cell: ({ row }) => (
          <Action
            action="open-review-detail"
            value={row.original.id}
            variant="subtle"
          >
            {row.original.username || "user"}
          </Action>
        ),
      },
      {
        accessorKey: "rating",
        header: "Оценка",
        size: 120,
        Cell: ({ cell }) => (
          <Group gap={5} wrap="nowrap">
            <PiStar color="var(--mantine-color-yellow-4)" />
            <Text size="sm">{cell.getValue()} / 5</Text>
          </Group>
        ),
      },
      {
        accessorKey: "comment",
        header: "Комментарий",
        size: 400,
        Cell: ({ cell }) => (
          <Text size="sm" lineClamp={2} style={{ whiteSpace: "normal" }}>
            {cell.getValue()}
          </Text>
        ),
      },
      {
        id: "actions",
        header: "Действия",
        enableColumnFilter: false,
        Cell: ({ row }) => (
          <Group gap={4} wrap="nowrap">
            <IconAction
              action="open-review-detail"
              value={row.original.id}
              label="Открыть отзыв"
            >
              <PiEye />
            </IconAction>
            {model.canDeleteReviews && (
              <IconAction
                action="admin-delete-review"
                value={row.original.id}
                label="Удалить отзыв"
                color="red"
              >
                <PiTrash />
              </IconAction>
            )}
          </Group>
        ),
      },
    ],
    [model.canDeleteReviews],
  );
  return (
    <Stack gap="md">
      <SimpleGrid cols={{ base: 1, sm: 2 }} spacing="xs">
        <Metric
          title="Всего отзывов"
          value={reviews.count || 0}
          icon={PiUsers}
          color="blue"
        />
        <Metric
          title="Средняя оценка"
          value={Number(reviews.average || 0).toLocaleString("ru", {
            maximumFractionDigits: 1,
          })}
          icon={PiStar}
          color="yellow"
        />
      </SimpleGrid>
      <Card p={0} className="rn-table-card">
        <DataTable
          id="reviews"
          columns={columns}
          data={reviews.items || []}
          extra={
            <Group gap="xs" p="xs">
              {model.canRewardReviews && (
                <Action action="open-review-rewards" leftSection={<PiGear />}>
                  Вознаграждение за отзыв
                </Action>
              )}
              {reviews.canCreate && (
                <Action action="open-review-compose" leftSection={<PiPlus />}>
                  Добавить отзыв
                </Action>
              )}
              {reviews.myReview && (
                <Action action="open-review-detail" value={reviews.myReview.id}>
                  Мой отзыв
                </Action>
              )}
              {reviews.myReview && !reviews.myReview.rewardGranted && (
                <Action action="retry-review-reward">
                  Получить вознаграждение
                </Action>
              )}
            </Group>
          }
        />
      </Card>
    </Stack>
  );
}
function Dialogs({ nodes, dispatch }) {
  return nodes.map((node, index) => {
    const sheet = node.querySelector(".modal__sheet");
    if (!sheet) return <Legacy key={index} node={node} />;
    const close = node.querySelector(".modal__backdrop[data-action]")?.dataset
      .action;
    const header = sheet.querySelector(".modal__header");
    const title =
      header?.querySelector(".modal__title")?.textContent ||
      header?.textContent ||
      "Настройки";
    const body = sheet.cloneNode(true);
    const bodyHeader = body.querySelector(".modal__header");
    if (bodyHeader) {
      bodyHeader.querySelector(".modal__title")?.remove();
      bodyHeader.querySelectorAll("button").forEach((button) => {
        if (button.dataset.action === close) button.remove();
      });
      if (bodyHeader.textContent.trim())
        bodyHeader.className = "rn-dialog-toolbar";
      else bodyHeader.remove();
    }
    return (
      <Modal
        key={node.dataset.supportModal || close || index}
        opened
        onClose={() => close && dispatch(close)}
        title={title}
        size={/banner|thread/.test(sheet.className) ? "xl" : "lg"}
        closeButtonProps={{ "aria-label": "Закрыть" }}
        overlayProps={{ backgroundOpacity: 0.65, blur: 3 }}
      >
        <Legacy node={body} />
      </Modal>
    );
  });
}
function Overview({ model }) {
  const search = document.createElement("div");
  search.innerHTML = model.searchHTML;
  return (
    <Stack gap="md">
      <Card>
        <Legacy node={search} />
      </Card>
      <SimpleGrid cols={{ base: 1, sm: 2, lg: 4 }}>
        {[
          [
            "Активные тарифы",
            model.plans.filter((p) => p.enabled).length,
            PiCreditCard,
          ],
          ["Пользователи", model.users.total ?? "—", PiUsers],
          ["Интеграции", model.integrations, PiLightning],
          ["Системные события", model.events.length, PiShieldCheck],
        ].map(([title, value, Icon]) => (
          <Metric key={title} title={title} value={value} icon={Icon} />
        ))}
      </SimpleGrid>
      <Card>
        <Title order={4} mb="md">
          Управление сервисом
        </Title>
        <SimpleGrid cols={{ base: 1, sm: 2, lg: 3 }}>
          {model.groups
            .flatMap((g) => g[2])
            .filter((r) =>
              [
                "users",
                "plans",
                "finance",
                "support",
                "broadcast",
                "integrations",
              ].includes(r[0]),
            )
            .map((r) => (
              <Action
                key={r[0]}
                action={
                  r[5] === "page" ? "admin-console-page" : "open-admin-section"
                }
                value={r[0]}
                variant="default"
                h={64}
                leftSection={glyph(r[3])}
              >
                {r[1]}
              </Action>
            ))}
        </SimpleGrid>
      </Card>
      <Card>
        <Group justify="space-between">
          <Title order={4}>Системные события</Title>
          <Action action="open-admin-section" value="diagnostics">
            Открыть журнал
          </Action>
        </Group>
        {model.events.length ? (
          model.events.slice(0, 5).map((e, i) => (
            <Text key={i} size="sm" mt="md">
              {e.operation} · {e.message}
            </Text>
          ))
        ) : (
          <Text c="dimmed" size="sm" mt="md">
            Нет открытых событий
          </Text>
        )}
      </Card>
    </Stack>
  );
}
function Admin({ model, content, dialogs, dispatch }) {
  const [mobile, setMobile] = useState(false),
    [preferences, setPreferences] = useState(false);
  const [sidebar, setSidebar] = useState(() => {
    try {
      return localStorage.getItem("admin.sidebar") === "true";
    } catch {
      return false;
    }
  });
  const active = model.page === "admin" ? model.section : model.page;
  const routes = model.groups.flatMap((g) => g[2]);
  const desktopGroups = [
    ["Пользователи", "Users", ["users", "administrators", "partners"], "users"],
    ["Ноды", "Nodes", ["servers"], "server"],
    [
      "Подписка",
      "Subscription",
      ["subscriptions", "plans", "subpage", "trial", "grace"],
      "grid",
    ],
    [
      "Коммерция",
      "Commerce",
      [
        "finance",
        "analytics",
        "promocodes",
        "referrals",
        "integrations",
        "moynalog",
      ],
      "cartShopping",
    ],
    [
      "Инструменты",
      "Tools",
      ["support", "reviews", "broadcast", "diagnostics", "ai"],
      "grid",
    ],
    [
      "Настройки",
      "Settings",
      [
        "content",
        "appearance",
        "layout",
        "localization",
        "features",
        "maintenance",
        "push",
        "status",
      ],
      "settings",
    ],
  ]
    .map(([ru, en, ids, icon]) => [
      ru,
      en,
      ids.map((id) => routes.find((r) => r[0] === id)).filter(Boolean),
      icon,
    ])
    .filter((g) => g[2].length);
  const route = model.groups.flatMap((g) => g[2]).find((r) => r[0] === active);
  const title =
    route?.[model.locale === "en" ? 2 : 1] ||
    (model.locale === "en" ? "Home" : "Главная");
  const navRoute = (r) => (
    <NavLink
      key={r[0]}
      component="button"
      type="button"
      label={r[model.locale === "en" ? 2 : 1]}
      leftSection={glyph(r[3])}
      active={r[0] === active}
      onClick={() => setMobile(false)}
      {...actionProps(
        r[5] === "page" ? "admin-console-page" : "open-admin-section",
        r[0],
      )}
    />
  );
  const navigation = (
    <>
      <NavLink
        component="button"
        type="button"
        label="Главная"
        leftSection={<PiStar size={18} />}
        active={active === "home"}
        {...actionProps("open-admin-section", "home")}
        onClick={() => setMobile(false)}
      />
      {model.groups.map((g) => (
        <div key={g[0]}>
          <Text className="rn-section-label">
            {g[model.locale === "en" ? 1 : 0]}
          </Text>
          {g[2].map(navRoute)}
          <Divider my="lg" variant="dashed" opacity={0.3} />
        </div>
      ))}
    </>
  );
  return (
    <MantineProvider theme={theme} forceColorScheme="dark">
      <div
        className={`app-shell rw-shell remna-admin ${sidebar ? "rn-sidebar-mode" : ""}`}
      >
        <header className="rn-header">
          <div className="rn-brand-row">
            <Group gap="sm">
              <ActionIcon
                className="rn-mobile-toggle"
                aria-label="Открыть меню"
                onClick={() => setMobile(true)}
              >
                <PiList size={22} />
              </ActionIcon>
              {model.logo ? (
                <img className="rn-brand-logo" src={model.logo} alt="" />
              ) : (
                <PiShieldCheck size={22} />
              )}
              <Text className="rn-brand" fw={700}>
                {model.brand}
              </Text>
            </Group>
            <Group gap="xs">
              <Tooltip label="Документация Remnawave" withinPortal={false}>
                <ActionIcon
                  component="a"
                  href="https://docs.rw/"
                  target="_blank"
                  rel="noopener noreferrer"
                  className="rn-header-control rn-header-secondary"
                  aria-label="Документация Remnawave"
                >
                  <TbBook size={22} />
                </ActionIcon>
              </Tooltip>
              <Tooltip label="Статус системы" withinPortal={false}>
                <ActionIcon
                  className="rn-header-control rn-header-secondary"
                  aria-label="Статус системы"
                  {...actionProps("open-admin-section", "status")}
                >
                  <TbChartBar size={22} />
                </ActionIcon>
              </Tooltip>
              <Tooltip label="Репозиторий Link-Bot" withinPortal={false}>
                <ActionIcon
                  component="a"
                  href="https://github.com/bruhxax/Link-Bot"
                  target="_blank"
                  rel="noopener noreferrer"
                  className="rn-header-control rn-header-secondary"
                  aria-label="Репозиторий Link-Bot"
                >
                  <TbBrandGithub size={22} />
                </ActionIcon>
              </Tooltip>
              <Tooltip label="Настройки интерфейса" withinPortal={false}>
                <ActionIcon
                  className="rn-header-control"
                  onClick={() => setPreferences(true)}
                  aria-label="Настройки интерфейса"
                >
                  <PiSlidersHorizontal size={22} />
                </ActionIcon>
              </Tooltip>
              <Menu position="bottom-end">
                <Menu.Target>
                  <ActionIcon
                    className="rn-header-control"
                    aria-label="Язык интерфейса"
                  >
                    <TbLanguage size={22} />
                  </ActionIcon>
                </Menu.Target>
                <Menu.Dropdown>
                  <Menu.Item {...actionProps("profile-language", "ru")}>
                    Русский
                  </Menu.Item>
                  <Menu.Item {...actionProps("profile-language", "en")}>
                    English
                  </Menu.Item>
                </Menu.Dropdown>
              </Menu>
              <IconAction
                action="admin-console-exit"
                label="Личный кабинет"
                className="rn-header-control"
              >
                <PiArrowSquareOut size={22} />
              </IconAction>
              {model.logout && (
                <IconAction
                  action="browser-logout"
                  label="Выйти"
                  className="rn-header-control"
                >
                  <PiSignOut size={22} />
                </IconAction>
              )}
            </Group>
          </div>
          {!sidebar && (
            <nav className="rn-nav">
              <button
                className={`rn-nav-item ${active === "home" ? "is-active" : ""}`}
                {...actionProps("open-admin-section", "home")}
              >
                <PiStar size={18} />
                Главная
              </button>
              {desktopGroups.map((g, i) => (
                <Menu key={g[0]} trigger="click-hover" position="bottom-start">
                  <Menu.Target>
                    <button
                      className={`rn-nav-item ${g[2].some((r) => r[0] === active) ? "is-active" : ""}`}
                    >
                      {glyph(g[3])}
                      {g[model.locale === "en" ? 1 : 0]}
                      <PiCaretDown size={12} />
                    </button>
                  </Menu.Target>
                  <Menu.Dropdown>
                    {g[2].map((r) => (
                      <Menu.Item
                        key={r[0]}
                        leftSection={glyph(r[3])}
                        {...actionProps(
                          r[5] === "page"
                            ? "admin-console-page"
                            : "open-admin-section",
                          r[0],
                        )}
                      >
                        {r[model.locale === "en" ? 2 : 1]}
                      </Menu.Item>
                    ))}
                  </Menu.Dropdown>
                </Menu>
              ))}
            </nav>
          )}
        </header>
        {sidebar && <aside className="rn-sidebar">{navigation}</aside>}
        <main className="page-scroll rn-main">
          <div className="rn-page">
            {active === "users" && <UserMetrics users={model.users} />}
            <Card className="rn-page-header" mb="md">
              <Group justify="space-between" wrap="wrap">
                <Group>
                  <ThemeIcon size="xl" variant="soft" radius="lg">
                    {active === "home" ? (
                      <PiStar size={24} />
                    ) : (
                      glyph(route?.[3])
                    )}
                  </ThemeIcon>
                  <Title order={4}>{title}</Title>
                </Group>
                <Group gap="xs">
                  {model.save && (
                    <Group className="rn-settings-actions" gap="xs">
                      <span role="status" className="rn-save-status">
                        {model.dirty ? "Есть несохранённые изменения" : ""}
                      </span>
                      <Action
                        action="admin-cancel-settings"
                        disabled={!model.dirty || model.busy}
                      >
                        Отменить
                      </Action>
                      <Action
                        action="admin-save-settings"
                        color="teal"
                        disabled={!model.dirty || model.busy}
                        leftSection={<PiCheck />}
                      >
                        Сохранить
                      </Action>
                    </Group>
                  )}
                  <IconAction action="refresh" label="Обновить">
                    <PiArrowsClockwise size={20} />
                  </IconAction>
                  {active === "plans" && (
                    <IconAction
                      action="admin-add-plan"
                      label="Добавить тариф"
                      color="teal"
                    >
                      <PiPlus size={22} />
                    </IconAction>
                  )}
                </Group>
              </Group>
            </Card>
            {active === "home" ? (
              <Overview model={model} />
            ) : active === "plans" ? (
              <Plans model={model} />
            ) : active === "layout" ? (
              <Layout model={model} />
            ) : active === "users" ? (
              <Users model={model} />
            ) : active === "servers" ? (
              <Nodes model={model} />
            ) : active === "reviews" ? (
              <Reviews model={model} />
            ) : (
              <div className="rn-content">
                <Legacy node={content} />
              </div>
            )}
          </div>
        </main>
        {active === "users" && model.userDetail && (
          <UserDialog
            model={model}
            content={content}
            dispatch={dispatch}
            Legacy={Legacy}
          />
        )}
        <Dialogs nodes={dialogs} dispatch={dispatch} />
        <Drawer
          opened={mobile}
          onClose={() => setMobile(false)}
          withinPortal={false}
          title={model.brand}
          size={300}
          className="rn-mobile-nav"
        >
          {navigation}
        </Drawer>
        <Modal
          opened={preferences}
          onClose={() => setPreferences(false)}
          title="Настройки интерфейса"
          size="md"
        >
          <Stack>
            <Text size="sm" c="dimmed">
              Настройки сохраняются в этом браузере.
            </Text>
            <Switch
              label="Боковое меню"
              description="Альтернативное расположение навигации"
              checked={sidebar}
              onChange={(e) => {
                setSidebar(e.currentTarget.checked);
                try {
                  localStorage.setItem(
                    "admin.sidebar",
                    String(e.currentTarget.checked),
                  );
                } catch {
                  /* Keep the session preference. */
                }
              }}
            />
            <Text size="sm">
              Плотность, порядок и видимость столбцов настраиваются в панели
              каждой таблицы.
            </Text>
            <Divider />
            <Text size="xs" c="dimmed">
              Интерфейс основан на Remnawave frontend · AGPL-3.0
            </Text>
            <Button
              component="a"
              href="/mini-app/admin-ui-source.zip"
              variant="subtle"
              download
            >
              Исходники интерфейса и лицензии
            </Button>
          </Stack>
        </Modal>
      </div>
    </MantineProvider>
  );
}
export function mountRemnaAdmin(element, model, shell) {
  const content =
    shell.querySelector(".page-scroll > .page") ||
    shell.querySelector(".page-scroll");
  const dialogs = [...shell.children].filter((n) =>
    n.matches(".modal,.admin-commerce-menu"),
  );
  const dispatch = (action, value) => {
    const button = document.createElement("button");
    button.dataset.action = action;
    if (value != null) button.dataset.value = value;
    element.append(button);
    button.click();
    button.remove();
  };
  if (!mountedRoot || mountedElement !== element) {
    mountedRoot = createRoot(element);
    mountedElement = element;
  }
  flushSync(() =>
    mountedRoot.render(
      <Admin
        model={model}
        content={content}
        dialogs={dialogs}
        dispatch={dispatch}
      />,
    ),
  );
}
export function unmountRemnaAdmin() {
  if (mountedRoot) {
    flushSync(() => mountedRoot.unmount());
    mountedRoot = null;
    mountedElement = null;
  }
}

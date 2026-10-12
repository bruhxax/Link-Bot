import React, { useMemo, useRef, useState } from "react";
import {
  ActionIcon,
  Badge,
  Button,
  Checkbox,
  CopyButton,
  Menu,
  Progress,
  Text,
  TextInput,
  Tooltip,
} from "@mantine/core";
import { DatePickerInput } from "@mantine/dates";
import "dayjs/locale/ru.js";
import {
  PiMagnifyingGlass,
  PiPulse,
  PiClockUser,
  PiProhibit,
  PiClockCountdown,
  PiCaretDown,
  PiCalendarBlank,
  PiDotsThreeVertical,
  PiDotsThree,
  PiPencilSimple,
  PiListBullets,
  PiCopy,
  PiDownload,
  PiCheck,
  PiArrowsClockwise,
} from "react-icons/pi";
import {
  expirationText,
  relativeTime,
  trafficOverview,
} from "./user-overview.mjs";
import {
  userStatuses,
  userStatus,
  userType,
  userInDateRange,
  usersCSV,
} from "./users-list-data.mjs";
import { SectionToolbar } from "./section-toolbar.jsx";

const types = [
  { value: "limited", label: "С лимитом" },
  { value: "unlimited", label: "Безлимит" },
  { value: "none", label: "Без подписки" },
  { value: "unknown", label: "Нет данных панели" },
];
const stamp = (value, withTime = false) =>
  value
    ? new Date(value).toLocaleString("ru", {
        day: "numeric",
        month: "short",
        year: "numeric",
        ...(withTime
          ? { hour: "2-digit", minute: "2-digit", second: "2-digit" }
          : {}),
      })
    : "—";

function FilterMenu({ label, value, items, onChange }) {
  return (
    <Menu position="bottom-end" withinPortal>
      <Menu.Target>
        <Button
          className="rn-users-filter-button"
          variant="default"
          rightSection={<PiCaretDown size={12} />}
          data-active={Boolean(value)}
          aria-label={label}
        >
          {label}
        </Button>
      </Menu.Target>
      <Menu.Dropdown>
        <Menu.Item
          leftSection={!value ? <PiCheck size={14} /> : null}
          onClick={() => onChange(undefined)}
        >
          Все
        </Menu.Item>
        {items.map((item) => (
          <Menu.Item
            key={item.value}
            leftSection={value === item.value ? <PiCheck size={14} /> : null}
            onClick={() => onChange(item.value)}
          >
            {item.label}
          </Menu.Item>
        ))}
      </Menu.Dropdown>
    </Menu>
  );
}

function Status({ value }) {
  const status = String(value).toLowerCase();
  const [color, Icon] = {
    active: ["teal", PiPulse],
    expired: ["red", PiClockUser],
    limited: ["orange", PiClockCountdown],
    disabled: ["gray", PiProhibit],
    blocked: ["red", PiProhibit],
  }[status] || ["gray", PiProhibit];
  return (
    <Badge
      className="rn-user-status"
      data-status={status}
      color={color}
      variant="outline"
      radius="sm"
      leftSection={<Icon size={11} />}
    >
      {status.toUpperCase()}
    </Badge>
  );
}

function Traffic({ user }) {
  const traffic = trafficOverview(user);
  if (!traffic) return <span className="rn-user-muted">Не загружено</span>;
  return (
    <Tooltip
      label={`Лимит: ${traffic.limit}. За всё время: ${traffic.lifetime}`}
    >
      <div className="rn-user-traffic">
        <div className="rn-user-traffic-top">
          <span>
            <b>{traffic.percent.toFixed(2)}%</b>{" "}
            <small>
              {traffic.strategy === "∞" ? "без сброса" : traffic.strategy}
            </small>
          </span>
          <Progress
            value={traffic.progress}
            size={5}
            radius="xl"
            color="#1dcca4"
            aria-label="Использование трафика"
          />
          <span className="rn-user-traffic-lifetime">
            Σ {traffic.lifetime}{" "}
            <em>
              {traffic.limit === "∞" ? "∞" : `${traffic.remaining.toFixed(2)}%`}
            </em>
          </span>
        </div>
        <span className="rn-user-muted">{traffic.used}</span>
      </div>
    </Tooltip>
  );
}

function RowActions({ user }) {
  return (
    <div className="rn-user-row-actions">
      <Tooltip label="Открыть карточку">
        <ActionIcon
          className="rn-user-edit"
          aria-label={`Редактировать пользователя ${user.panelUsername || user.username || user.customerId}`}
          data-action="admin-user-open"
          data-value={user.customerId}
        >
          <PiPencilSimple size={13} />
        </ActionIcon>
      </Tooltip>
      <Menu position="bottom-end" withinPortal>
        <Menu.Target>
          <ActionIcon
            variant="subtle"
            aria-label={`Действия пользователя ${user.customerId}`}
          >
            <PiDotsThree size={17} />
          </ActionIcon>
        </Menu.Target>
        <Menu.Dropdown>
          <Menu.Item data-action="admin-user-open" data-value={user.customerId}>
            Открыть карточку
          </Menu.Item>
          <CopyButton value={String(user.telegramId)}>
            {({ copy, copied }) => (
              <Menu.Item onClick={copy}>
                {copied ? "Скопировано" : "Скопировать Telegram ID"}
              </Menu.Item>
            )}
          </CopyButton>
          {user.subscriptionLink && (
            <CopyButton value={user.subscriptionLink}>
              {({ copy, copied }) => (
                <Menu.Item onClick={copy}>
                  {copied ? "Скопировано" : "Скопировать ссылку подписки"}
                </Menu.Item>
              )}
            </CopyButton>
          )}
        </Menu.Dropdown>
      </Menu>
    </div>
  );
}

function downloadRows(table) {
  const visible = table.getRowModel().rows;
  const selected = visible.filter((row) => row.getIsSelected());
  const rows = selected.length ? selected : visible;
  const url = URL.createObjectURL(
    new Blob(["\uFEFF", usersCSV(rows.map((row) => row.original))], {
      type: "text/csv;charset=utf-8",
    }),
  );
  const link = document.createElement("a");
  link.href = url;
  link.download = `users-${new Date().toISOString().slice(0, 10)}.csv`;
  link.click();
  setTimeout(() => URL.revokeObjectURL(url), 1000);
}

function ListToolbar({ model, table }) {
  const plans = [
    ...new Set(
      table.options.data.map((user) => user.planTitle).filter(Boolean),
    ),
  ]
    .sort()
    .map((value) => ({ value, label: value }));
  const filter = (id) => table.getColumn(id);
  const visible = table.getRowModel().rows;
  const selected = visible.filter((row) => row.getIsSelected());
  const rows = selected.length ? selected : visible;
  const copyLabel = selected.length
    ? "Скопировать выбранных пользователей"
    : "Скопировать текущий список";
  const downloadLabel = selected.length
    ? "Скачать выбранных пользователей CSV"
    : "Скачать текущий список CSV";
  return (
    <div className="rn-users-toolbar">
      <TextInput
        className="rn-users-search"
        placeholder="Имя, Telegram ID или подписка"
        aria-label="Найти пользователя"
        data-input="admin-users-search"
        defaultValue={model.usersQuery}
        leftSection={<PiMagnifyingGlass size={16} />}
      />
      <div className="rn-users-toolbar-actions">
        <FilterMenu
          label="Статус"
          value={filter("status")?.getFilterValue()}
          items={userStatuses}
          onChange={(value) => filter("status")?.setFilterValue(value)}
        />
        <FilterMenu
          label="Тариф"
          value={filter("plan")?.getFilterValue()}
          items={plans}
          onChange={(value) => filter("plan")?.setFilterValue(value)}
        />
        <FilterMenu
          label="Тип"
          value={filter("type")?.getFilterValue()}
          items={types}
          onChange={(value) => filter("type")?.setFilterValue(value)}
        />
        <Menu position="bottom-end" withinPortal>
          <Menu.Target>
            <Button
              className="rn-users-filter-button"
              variant="default"
              rightSection={<PiCaretDown size={12} />}
              aria-label="Сортировка по дате создания"
            >
              Создан
            </Button>
          </Menu.Target>
          <Menu.Dropdown>
            <Menu.Item
              onClick={() =>
                table.setSorting([{ id: "createdAt", desc: true }])
              }
            >
              Сначала новые
            </Menu.Item>
            <Menu.Item
              onClick={() =>
                table.setSorting([{ id: "createdAt", desc: false }])
              }
            >
              Сначала старые
            </Menu.Item>
          </Menu.Dropdown>
        </Menu>
        <Menu position="bottom-end" withinPortal>
          <Menu.Target>
            <ActionIcon
              className="rn-users-tool"
              aria-label="Настроить таблицу"
            >
              <PiListBullets size={18} />
            </ActionIcon>
          </Menu.Target>
          <Menu.Dropdown>
            <Menu.Label>Столбцы</Menu.Label>
            {table
              .getAllLeafColumns()
              .filter((column) => column.getCanHide())
              .map((column) => (
                <Menu.Item
                  key={column.id}
                  closeMenuOnClick={false}
                  onClick={() => column.toggleVisibility()}
                  leftSection={
                    <Checkbox
                      size="xs"
                      checked={column.getIsVisible()}
                      readOnly
                      tabIndex={-1}
                    />
                  }
                >
                  {column.columnDef.header}
                </Menu.Item>
              ))}
            <Menu.Divider />
            <Menu.Label>Плотность строк</Menu.Label>
            {[
              ["xxs", "Компактно"],
              ["xs", "Обычно"],
              ["md", "Свободно"],
            ].map(([value, label]) => (
              <Menu.Item
                key={value}
                onClick={() => table.setDensity(value)}
                leftSection={
                  table.getState().density === value ? (
                    <PiCheck size={14} />
                  ) : null
                }
              >
                {label}
              </Menu.Item>
            ))}
            <Menu.Divider />
            <Menu.Item
              onClick={() =>
                table.setShowColumnFilters(!table.getState().showColumnFilters)
              }
            >
              Фильтры по столбцам
            </Menu.Item>
          </Menu.Dropdown>
        </Menu>
        <CopyButton value={usersCSV(rows.map((row) => row.original))}>
          {({ copy, copied }) => (
            <Tooltip label={copied ? "Скопировано" : copyLabel}>
              <ActionIcon
                className="rn-users-tool"
                aria-label={copyLabel}
                onClick={copy}
              >
                {copied ? <PiCheck size={18} /> : <PiCopy size={18} />}
              </ActionIcon>
            </Tooltip>
          )}
        </CopyButton>
        <Tooltip label={downloadLabel}>
          <ActionIcon
            className="rn-users-tool"
            aria-label={downloadLabel}
            onClick={() => downloadRows(table)}
          >
            <PiDownload size={18} />
          </ActionIcon>
        </Tooltip>
      </div>
    </div>
  );
}

export function Users({ model, DataTable, Metrics, hidden = false }) {
  const tableRef = useRef(null);
  const [range, setRange] = useState([null, null]);
  const [tab, setTab] = useState("users-overview");
  const columns = useMemo(
    () => [
      {
        id: "name",
        header: "Имя",
        size: 158,
        minSize: 110,
        accessorFn: (user) =>
          user.panelUsername || user.username || String(user.telegramId),
        Cell: ({ row, cell }) => {
          const user = row.original,
            online =
              user.onlineAt && Date.now() - Date.parse(user.onlineAt) < 120000;
          return (
            <UnstyledName user={user} value={cell.getValue()} online={online} />
          );
        },
      },
      {
        id: "panelId",
        header: "ID",
        size: 58,
        minSize: 50,
        accessorFn: (user) => user.panelId || user.customerId,
        Cell: ({ cell }) => (
          <span className="rn-user-id">{cell.getValue()}</span>
        ),
      },
      {
        id: "status",
        header: "Статус",
        size: 95,
        minSize: 85,
        accessorFn: userStatus,
        filterVariant: "select",
        filterSelectOptions: userStatuses,
        filterFn: "equals",
        Cell: ({ cell }) => <Status value={cell.getValue()} />,
      },
      {
        id: "plan",
        header: "Тариф",
        size: 118,
        minSize: 85,
        accessorFn: (user) => user.planTitle || "",
        filterFn: "equals",
        Cell: ({ cell }) => <span>{cell.getValue() || "—"}</span>,
      },
      {
        accessorKey: "expiresAt",
        header: "Истекает",
        size: 116,
        minSize: 95,
        enableColumnFilter: false,
        Cell: ({ cell }) => (
          <Tooltip label={stamp(cell.getValue(), true)}>
            <span className="rn-user-muted">
              {expirationText(cell.getValue()).replace(/^Истекает /, "")}
            </span>
          </Tooltip>
        ),
      },
      {
        id: "traffic",
        header: "Использовано",
        size: 310,
        minSize: 230,
        enableColumnFilter: false,
        accessorFn: (user) =>
          user.trafficLoaded ? user.usedTrafficBytes : null,
        Cell: ({ row }) => <Traffic user={row.original} />,
      },
      {
        accessorKey: "tag",
        header: "Тег",
        size: 92,
        minSize: 65,
        Cell: ({ cell }) => (
          <span className="rn-user-tag">{cell.getValue() || "—"}</span>
        ),
      },
      {
        id: "createdAt",
        header: "Создан",
        size: 106,
        minSize: 95,
        accessorFn: (user) => user.panelCreatedAt || user.createdAt,
        filterFn: (row, id, value) => userInDateRange(row.getValue(id), value),
        Cell: ({ cell }) => (
          <Tooltip label={stamp(cell.getValue(), true)}>
            <span className="rn-user-created">{stamp(cell.getValue())}</span>
          </Tooltip>
        ),
      },
      {
        accessorKey: "onlineAt",
        header: "Последний вход",
        size: 170,
        minSize: 150,
        Cell: ({ cell }) => (
          <Tooltip label="Последнее подключение в панели">
            <span className="rn-user-created">
              {stamp(cell.getValue(), true)}
            </span>
          </Tooltip>
        ),
      },
      {
        accessorKey: "description",
        header: "Описание",
        size: 170,
        Cell: ({ cell }) => (
          <Tooltip
            label={cell.getValue() || "Без описания"}
            multiline
            maw={320}
          >
            <Text size="xs" lineClamp={2}>
              {cell.getValue() || "—"}
            </Text>
          </Tooltip>
        ),
      },
      {
        id: "type",
        header: "Тип",
        size: 110,
        accessorFn: userType,
        filterFn: "equals",
        Cell: ({ cell }) =>
          types.find((item) => item.value === cell.getValue())?.label || "—",
      },
      {
        id: "actions",
        header: "Действия",
        size: 72,
        minSize: 64,
        enableColumnFilter: false,
        enableSorting: false,
        enableHiding: false,
        Cell: ({ row }) => <RowActions user={row.original} />,
      },
    ],
    [],
  );
  return (
    <div className="rn-users-section">
      <SectionToolbar
        model={model}
        active="users"
        userTab={tab}
        setUserTab={setTab}
      >
        <div className="rn-users-period-actions">
          <DatePickerInput
            type="range"
            className="rn-users-period"
            aria-label="Период создания пользователей"
            placeholder="Весь период"
            valueFormat="D MMM YYYY"
            locale="ru"
            leftSection={<PiCalendarBlank size={16} />}
            value={range}
            clearable
            onChange={(value) => {
              setRange(value);
              tableRef.current
                ?.getColumn("createdAt")
                ?.setFilterValue(value.some(Boolean) ? value : undefined);
            }}
          />
          <Menu position="bottom-end" withinPortal>
            <Menu.Target>
              <ActionIcon
                className="rn-users-period-menu"
                aria-label="Действия раздела пользователей"
              >
                <PiDotsThreeVertical size={19} />
              </ActionIcon>
            </Menu.Target>
            <Menu.Dropdown>
              <Menu.Item
                leftSection={<PiArrowsClockwise size={15} />}
                data-action="refresh"
                disabled={model.userBusy}
              >
                Обновить пользователей
              </Menu.Item>
              <Menu.Item
                onClick={() => {
                  tableRef.current?.setColumnFilters([]);
                  setRange([null, null]);
                }}
              >
                Сбросить фильтры
              </Menu.Item>
            </Menu.Dropdown>
          </Menu>
        </div>
      </SectionToolbar>
      {tab === "users-overview" && (
        <Metrics
          users={model.users}
          hidden={hidden}
          overview={model.overview}
        />
      )}
      <h2 className="rn-users-list-title">Список пользователей</h2>
      <div className="rn-users-table" aria-busy={model.userBusy}>
        <DataTable
          columns={columns}
          data={model.users.items || []}
          id="users-reference-v1"
          paginate={false}
          variant="users"
          loading={model.userBusy}
          onTable={(table) => {
            tableRef.current = table;
          }}
          extra={(table) => <ListToolbar table={table} model={model} />}
        />
        <div className="rn-users-footer">
          <div>
            <span>
              Загружено:{" "}
              <span className="rn-private-value" aria-hidden={hidden}>
                {model.users.items.length} из{" "}
                {Number(model.users.total).toLocaleString("ru")}
              </span>
            </span>
            <small>Фильтры, даты и сортировка — по загруженному списку</small>
          </div>
          {model.users.items.length < model.users.total && (
            <Button
              variant="default"
              data-action="admin-users-more"
              disabled={model.userBusy}
              loading={model.userBusy}
            >
              Показать ещё
            </Button>
          )}
        </div>
      </div>
    </div>
  );
}

function UnstyledName({ user, value, online }) {
  return (
    <Tooltip
      label={
        user.trafficLoaded
          ? user.onlineAt
            ? relativeTime(user.onlineAt)
            : "Ещё не подключался"
          : "Нет данных панели"
      }
    >
      <button
        type="button"
        className="rn-user-name"
        data-action="admin-user-open"
        data-value={user.customerId}
        aria-label={`Открыть пользователя ${value}`}
      >
        <span
          className="rn-connection-dot"
          data-status={online ? "online" : user.onlineAt ? "offline" : "never"}
        />
        <span>{value}</span>
      </button>
    </Tooltip>
  );
}

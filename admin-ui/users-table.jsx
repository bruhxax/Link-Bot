import React, { useMemo } from "react";
import {
  Badge,
  Card,
  Group,
  Progress,
  Stack,
  Text,
  TextInput,
  Tooltip,
  UnstyledButton,
  Button,
} from "@mantine/core";
import {
  PiMagnifyingGlass,
  PiPulse,
  PiClockUser,
  PiProhibit,
  PiClockCountdown,
} from "react-icons/pi";
import {
  expirationText,
  relativeTime,
  trafficOverview,
} from "./user-overview.mjs";

function Traffic({ user }) {
  const traffic = trafficOverview(user);
  if (!traffic)
    return (
      <Text size="xs" c="dimmed">
        Не загружено
      </Text>
    );
  return (
    <div className="rn-user-traffic">
      <Group justify="space-between" gap={6} wrap="nowrap">
        <Text size="xs" c="red.5" fw={600}>
          {traffic.percent.toFixed(2)}%{" "}
          <Text component="span" size="xs" c="dimmed" fw={400}>
            {traffic.strategy}
          </Text>
        </Text>
        <Text size="xs" c="teal.5" fw={600}>
          <Text component="span" size="xs" c="dimmed" fw={400}>
            Σ {traffic.lifetime}
          </Text>{" "}
          {traffic.remaining.toFixed(2)}%
        </Text>
      </Group>
      <Progress
        radius="xs"
        size={5}
        value={traffic.progress}
        color={traffic.color}
      />
      <Group justify="space-between" gap={6} wrap="nowrap" mt={2}>
        <Text size="xs" c="dimmed">
          {traffic.used}
        </Text>
        <Text size="xs" c="dimmed">
          {traffic.limit}
        </Text>
      </Group>
    </div>
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
      color={color}
      variant="light"
      radius="sm"
      leftSection={<Icon size={13} />}
    >
      {status.toUpperCase()}
    </Badge>
  );
}

export function Users({ model, DataTable }) {
  const columns = useMemo(
    () => [
      {
        id: "name",
        header: "Имя",
        size: 170,
        accessorFn: (u) =>
          u.panelUsername || u.username || String(u.telegramId),
        Cell: ({ row, cell }) => {
          const user = row.original;
          const online =
            user.onlineAt && Date.now() - Date.parse(user.onlineAt) < 120000;
          return (
            <UnstyledButton
              className="rn-user-name"
              data-action="admin-user-open"
              data-value={user.customerId}
            >
              <span
                className="rn-connection-dot"
                data-status={
                  online ? "online" : user.onlineAt ? "offline" : "never"
                }
              />
              <Stack gap={0} style={{ minWidth: 0 }}>
                <Text size="sm" fw={500} truncate>
                  {cell.getValue()}
                </Text>
                <Text size="xs" c="dimmed">
                  {user.trafficLoaded
                    ? user.onlineAt
                      ? relativeTime(user.onlineAt)
                      : "Ещё не подключался"
                    : "Нет данных панели"}
                </Text>
              </Stack>
            </UnstyledButton>
          );
        },
      },
      {
        id: "panelId",
        header: "ID",
        size: 110,
        minSize: 110,
        accessorFn: (u) => u.panelId || u.customerId,
        Cell: ({ cell }) => (
          <Text size="xs" className="rn-number">
            {cell.getValue()}
          </Text>
        ),
      },
      {
        id: "status",
        header: "Статус",
        size: 130,
        accessorFn: (u) =>
          u.isBlocked ? "blocked" : u.subscriptionStatus || "none",
        filterVariant: "select",
        Cell: ({ cell }) => <Status value={cell.getValue()} />,
      },
      {
        accessorKey: "expiresAt",
        header: "Истекает",
        size: 190,
        enableColumnFilter: false,
        Cell: ({ cell }) => (
          <Tooltip
            label={
              cell.getValue()
                ? new Date(cell.getValue()).toLocaleString("ru")
                : "Без подписки"
            }
          >
            <Text size="xs" c="dimmed">
              {expirationText(cell.getValue())}
            </Text>
          </Tooltip>
        ),
      },
      {
        id: "traffic",
        header: "Израсходовано",
        size: 280,
        enableColumnFilter: false,
        accessorFn: (u) => (u.trafficLoaded ? u.usedTrafficBytes : null),
        Cell: ({ row }) => <Traffic user={row.original} />,
      },
      {
        accessorKey: "description",
        header: "Описание",
        size: 220,
        Cell: ({ cell }) => (
          <Text size="xs" className="rn-user-description">
            {cell.getValue() || "—"}
          </Text>
        ),
      },
      {
        accessorKey: "tag",
        header: "Тег",
        size: 100,
        filterVariant: "select",
        Cell: ({ cell }) => (
          <Text size="xs" fw={600}>
            {cell.getValue() || "—"}
          </Text>
        ),
      },
      {
        id: "createdAt",
        header: "Создан",
        size: 210,
        accessorFn: (u) => u.panelCreatedAt || u.createdAt,
        Cell: ({ cell }) => (
          <Text size="xs" className="rn-number">
            {cell.getValue()
              ? new Date(cell.getValue()).toLocaleString("ru", {
                  hour: "2-digit",
                  minute: "2-digit",
                  second: "2-digit",
                  day: "numeric",
                  month: "long",
                  year: "numeric",
                })
              : "—"}
          </Text>
        ),
      },
    ],
    [],
  );
  return (
    <Card p={0} className="rn-table-card rn-users-table">
      <DataTable
        columns={columns}
        data={model.users.items || []}
        id="users-panel-v2"
        paginate={false}
        extra={
          <TextInput
            m="xs"
            w={320}
            maw="100%"
            placeholder="Имя, Telegram ID или подписка"
            aria-label="Найти пользователя"
            data-input="admin-users-search"
            defaultValue={model.usersQuery}
            leftSection={<PiMagnifyingGlass size={16} />}
          />
        }
      />
      {model.users.items?.length < model.users.total && (
        <Group justify="center" p="sm">
          <Text size="xs" c="dimmed">
            Загружено {model.users.items.length} из{" "}
            {Number(model.users.total).toLocaleString("ru")}
          </Text>
          <Button data-action="admin-users-more">Показать ещё</Button>
        </Group>
      )}
    </Card>
  );
}

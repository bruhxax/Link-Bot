import React, { useRef } from "react";
import {
  ActionIcon,
  Badge,
  Button,
  Card,
  Group,
  Menu,
  SimpleGrid,
  Stack,
  Switch,
  Text,
  TextInput,
  Title,
  Tooltip,
} from "@mantine/core";
import { TbArrowsMove, TbPlus, TbSettings } from "react-icons/tb";

export function LayoutEditor({ model }) {
  const areas = [...new Set(model.layoutEntries.map((e) => e.area))];
  const area = areas.includes(model.layoutArea)
    ? model.layoutArea
    : areas[0] || "dashboard";
  const names = {
    dashboard: "Главный экран",
    buy: "Покупка",
    support: "Поддержка",
    profile: "Профиль",
    navigation: "Навигация",
    settings: "Профиль",
  };
  const labels = {
    logo: "Логотип",
    username: "Имя пользователя",
    plan_name: "Название тарифа",
    expires: "Дата окончания",
  };
  const rows = model.layoutEntries.filter((e) => e.area === area);
  const pointer = useRef(null);
  const begin = (e, item) => {
    if (e.button !== 0) return;
    e.preventDefault();
    e.currentTarget.setPointerCapture(e.pointerId);
    pointer.current = {
      id: e.pointerId,
      key: `${area}:${item.id}`,
      element: e.currentTarget,
      x: e.clientX,
      y: e.clientY,
      startX: Number(item.positionX || 0),
      startY: Number(item.positionY || 0),
    };
  };
  const move = (e) => {
    const p = pointer.current;
    if (p?.id === e.pointerId)
      p.element.style.transform = `translate(${e.clientX - p.x}px,${e.clientY - p.y}px)`;
  };
  const end = (e) => {
    const p = pointer.current;
    if (p?.id !== e.pointerId) return;
    pointer.current = null;
    p.element.style.transform = "";
    model.moveLayout(
      p.key,
      p.startX + e.clientX - p.x,
      p.startY + e.clientY - p.y,
    );
  };
  const cancel = (e) => {
    if (pointer.current?.id === e.pointerId) {
      pointer.current.element.style.transform = "";
      pointer.current = null;
    }
  };
  return (
    <Stack gap="md">
      <Group justify="space-between">
        <Group gap="xs">
          {areas.map((a) => (
            <Button
              key={a}
              variant={area === a ? "filled" : "light"}
              onClick={() => model.setLayoutArea(a)}
            >
              {names[a] || a}
            </Button>
          ))}
        </Group>
        <Menu position="bottom-end">
          <Menu.Target>
            <Button leftSection={<TbPlus />}>Добавить</Button>
          </Menu.Target>
          <Menu.Dropdown>
            {area === "dashboard" ? (
              <>
                <Menu.Item data-action="admin-add-notification-widget">
                  Уведомление
                </Menu.Item>
                <Menu.Item data-action="admin-add-promo-widget">
                  Подарок с промокодом
                </Menu.Item>
                <Menu.Item data-action="admin-add-banner">
                  Баннер · PNG, GIF или MP4
                </Menu.Item>
                <Menu.Item data-action="admin-add-empty-card">
                  Пустая карточка
                </Menu.Item>
              </>
            ) : area === "profile" || area === "settings" ? (
              <Menu.Item data-action="admin-add-profile-button">
                Кнопка профиля
              </Menu.Item>
            ) : (
              <Menu.Label>Элементы настраиваются ниже</Menu.Label>
            )}
            <Menu.Divider />
            <Menu.Item data-action="admin-layout-reset-category">
              Сбросить экран
            </Menu.Item>
          </Menu.Dropdown>
        </Menu>
      </Group>
      <Text size="sm" c="dimmed">
        Настройте положение, размеры и видимость элементов. Перетаскивание
        крестика изменяет координаты X и Y. Изменения применяются после
        сохранения.
      </Text>
      {rows.map((item) => (
        <Card key={`${area}:${item.id}`}>
          <Group justify="space-between" mb="md">
            <Group gap="sm">
              <Tooltip label="Переместить элемент" withinPortal={false}>
                <ActionIcon
                  variant="subtle"
                  aria-label={`Переместить: ${labels[item.id] || item.label}`}
                  style={{ touchAction: "none", cursor: "grab" }}
                  onPointerDown={(e) => begin(e, item)}
                  onPointerMove={move}
                  onPointerUp={end}
                  onPointerCancel={cancel}
                >
                  <TbArrowsMove size={20} />
                </ActionIcon>
              </Tooltip>
              <Title order={5}>{labels[item.id] || item.label}</Title>
              <Badge variant="light" color="gray">
                {item.id}
              </Badge>
            </Group>
            <Group>
              <Switch
                key={String(item.visible)}
                label="Показывать"
                defaultChecked={item.visible !== false}
                data-setting-path={`layout.elements.${item.index}.visible`}
                data-setting-type="boolean"
              />
              {area === "dashboard" && (
                <Button
                  leftSection={<TbSettings />}
                  onClick={() => model.editLayout(`${area}:${item.id}`)}
                >
                  Оформление
                </Button>
              )}
              {(area === "profile" || area === "settings") &&
                model.profileButtons.some((p) => p.id === item.id) && (
                  <Button onClick={() => model.editProfileButton(item.id)}>
                    Изменить кнопку
                  </Button>
                )}
            </Group>
          </Group>
          <SimpleGrid cols={{ base: 2, sm: 3, lg: 6 }}>
            {[
              ["positionX", "Позиция X"],
              ["positionY", "Позиция Y"],
              ["width", area === "navigation" ? "Ширина, px" : "Ширина, %"],
              ["height", "Высота, px"],
              ["order", "Порядок"],
              ["layer", "Слой"],
            ].map(([field, label]) => (
              <TextInput
                key={`${field}:${item[field]}`}
                label={label}
                type="number"
                defaultValue={item[field] ?? 0}
                data-setting-path={`layout.elements.${item.index}.${field}`}
                data-setting-type="number"
              />
            ))}
          </SimpleGrid>
        </Card>
      ))}
      {(area === "profile" || area === "settings") && (
        <Card>
          <Title order={5} mb="md">
            Кнопки и документы профиля
          </Title>
          <Stack gap="xs">
            {model.profileButtons.map((p) => (
              <Group key={p.id} justify="space-between">
                <Text size="sm">{p.label}</Text>
                <Button onClick={() => model.editProfileButton(p.id)}>
                  Редактировать
                </Button>
              </Group>
            ))}
          </Stack>
        </Card>
      )}
    </Stack>
  );
}

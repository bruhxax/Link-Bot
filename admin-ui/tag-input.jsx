// Adapted from Remnawave CreateableTagInputShared, AGPL-3.0.
import React, { useState } from "react";
import { CloseButton, Combobox, InputBase, useCombobox } from "@mantine/core";
import { PiTagDuotone } from "react-icons/pi";

export function TagInput({ value, onChange, tags = [], disabled }) {
  const [initial] = useState(value);
  const [created, setCreated] = useState([]),
    [error, setError] = useState("");
  const store = useCombobox({
    onDropdownClose: () => store.resetSelectedOption(),
  });
  const data = [...new Set([...tags, ...created, initial].filter(Boolean))];
  const exact = data.includes(value);
  const choices = exact
    ? data
    : data.filter((tag) =>
        tag.toLowerCase().includes((value || "").trim().toLowerCase()),
      );
  const valid = (tag) => !tag || /^[A-Z0-9_]{1,16}$/.test(tag);
  return (
    <Combobox
      store={store}
      position="top"
      withinPortal={false}
      onOptionSubmit={(option) => {
        if (option === "$create") {
          if (!valid(value)) {
            setError("До 16 символов: A–Z, 0–9 и _");
            return;
          }
          setCreated((tags) => [...tags, value]);
        } else onChange(option);
        setError("");
        store.closeDropdown();
      }}
    >
      <Combobox.Target>
        <InputBase
          label="Тег"
          description="Создайте или выберите тег"
          placeholder="EXAMPLE_TAG_1"
          leftSection={<PiTagDuotone size={16} />}
          disabled={disabled}
          value={value || ""}
          error={error}
          onClick={() => store.openDropdown()}
          onFocus={() => store.openDropdown()}
          onBlur={() => {
            store.closeDropdown();
            setError(valid(value) ? "" : "До 16 символов: A–Z, 0–9 и _");
          }}
          onChange={(e) => {
            onChange(e.currentTarget.value);
            store.openDropdown();
            store.updateSelectedOptionIndex();
          }}
          rightSection={
            value ? (
              <CloseButton
                aria-label="Очистить тег"
                size="sm"
                onMouseDown={(e) => e.preventDefault()}
                onClick={() => {
                  onChange("");
                  setError("");
                }}
              />
            ) : (
              <Combobox.Chevron />
            )
          }
          rightSectionPointerEvents={value ? "all" : "none"}
        />
      </Combobox.Target>
      <Combobox.Dropdown>
        <Combobox.Options mah={200} style={{ overflowY: "auto" }}>
          {choices.map((tag) => (
            <Combobox.Option key={tag} value={tag}>
              {tag}
            </Combobox.Option>
          ))}
          {!exact && value?.trim() && (
            <Combobox.Option value="$create">+ {value}</Combobox.Option>
          )}
          {!choices.length && !value && (
            <Combobox.Empty>Введите новый тег</Combobox.Empty>
          )}
        </Combobox.Options>
      </Combobox.Dropdown>
    </Combobox>
  );
}

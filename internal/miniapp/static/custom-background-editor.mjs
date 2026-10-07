// Markup stays separate from media playback so slider input never restarts a clip.
export function renderCustomBackgroundEditor(appearance, helpers, busy = false, urlDraft = "") {
  const { html, attr, icon, range, text } = helpers;
  const items = appearance.customBackgrounds || [];
  const item = items.find(item => item.id === appearance.activeBackground);
  const index = items.indexOf(item);
  const path = `appearance.customBackgrounds.${index}`;
  const label = (ru, en) => html(text(ru, en));
  return `<section class="admin-editor__section custom-bg-editor">
    <div class="custom-bg-editor__heading"><h3>${label("Свои фоны", "Custom backgrounds")}</h3><small>${items.length}/20</small></div>
    <div class="custom-bg-add"><label class="custom-bg-upload ${busy ? "is-busy" : ""}">${icon("paperclip")}<span>${label(busy ? "Загрузка…" : "Загрузить файл", busy ? "Uploading…" : "Upload a file")}</span><input type="file" accept="image/*,video/*,.m4v,.ogv" data-input="admin-background-file" ${busy || items.length >= 20 ? "disabled" : ""}></label>
      <div class="custom-bg-url"><input type="url" placeholder="https://…" aria-label="${attr(text("Ссылка на фон", "Background URL"))}" data-input="admin-background-url" value="${attr(urlDraft)}" ${busy ? "disabled" : ""}><button type="button" data-action="admin-background-import" aria-label="${attr(text("Добавить по ссылке", "Add from URL"))}" ${busy || items.length >= 20 ? "disabled" : ""}>${icon("plus")}</button></div>
    </div>
    <p class="custom-bg-help">${label("Изображения, GIF и видео · до 50 МБ. По ссылке — прямой адрес файла.", "Images, GIF and video · up to 50 MB. Use a direct file URL.")}</p>
    ${items.length ? `<div class="custom-bg-library" role="radiogroup" aria-label="${attr(text("Сохранённые фоны", "Saved backgrounds"))}">${items.map(entry => `<button type="button" class="custom-bg-thumb ${entry.id === item?.id ? "is-selected" : ""}" role="radio" aria-checked="${entry.id === item?.id}" data-action="admin-background-select" data-value="${attr(entry.id)}" title="${attr(entry.name)}">${entry.type === "video" ? `<video src="${attr(entry.url)}#t=0.1" muted playsinline preload="metadata"></video>` : `<img src="${attr(entry.poster || entry.url)}" alt="" loading="lazy">`}<span>${html(entry.name)}</span></button>`).join("")}</div>` : `<div class="custom-bg-empty">${icon("image")}<span>${label("Добавьте свой первый фон", "Add your first background")}</span></div>`}
    ${item ? `<div class="custom-bg-selected"><input type="text" value="${attr(item.name)}" maxlength="100" aria-label="${attr(text("Название фона", "Background name"))}" data-setting-path="${path}.name"><button type="button" data-action="admin-background-remove" data-value="${attr(item.id)}" aria-label="${attr(text("Удалить фон", "Remove background"))}">${icon("trash")}</button></div>
    <div class="custom-bg-preview-wrap"><div class="custom-bg-preview custom-background" data-custom-bg-preview tabindex="0" role="img" aria-label="${attr(text("Предпросмотр фона. Перетаскивайте для изменения положения", "Background preview. Drag to reposition"))}"><span class="custom-background__shade"></span></div></div>
    <p class="custom-bg-error" data-custom-bg-error role="status"></p>
    <div class="custom-bg-preview-actions"><small>${label("Двигайте фон пальцем или мышью", "Drag with your finger or mouse")}</small><button type="button" data-action="admin-background-reset">${icon("reset")}${label("Сбросить", "Reset")}</button></div>
    <label class="custom-bg-fit"><span>${label("Размер", "Fit")}</span><select aria-label="${attr(text("Размер", "Fit"))}" data-setting-path="${path}.fit"><option value="cover" ${item.fit === "cover" ? "selected" : ""}>${label("Заполнить экран", "Fill screen")}</option><option value="contain" ${item.fit === "contain" ? "selected" : ""}>${label("Показать целиком", "Fit inside")}</option></select></label>
    <div class="custom-bg-ranges">
      ${range(text("Масштаб", "Scale"), "", `${path}.scale`, { value:item.scale, min:50, max:300, suffix:"%" })}
      ${range(text("Затемнение", "Dimming"), "", `${path}.dimming`, { value:item.dimming, min:0, max:90, suffix:"%" })}
      ${item.type !== "image" ? range(text("Скорость", "Speed"), text("100% — оригинальная скорость", "100% — original speed"), `${path}.speed`, { value:item.speed, min:10, max:200, suffix:"%" }) : ""}
    </div>
    <details class="custom-bg-position"><summary>${label("Точное положение", "Precise position")}</summary><div class="custom-bg-ranges">
      ${range(text("По горизонтали", "Horizontal"), "", `${path}.positionX`, { value:item.positionX, min:0, max:100, suffix:"%" })}
      ${range(text("По вертикали", "Vertical"), "", `${path}.positionY`, { value:item.positionY, min:0, max:100, suffix:"%" })}
    </div></details>` : ""}
  </section>`;
}

export function renderAISettings(state, { escapeHtml: html, escapeAttribute: attr, icon }) {
	const draft = state.draft;
	if (!draft) return state.error ? `<div class="admin-ai__loading"><p class="admin-ai__error" role="alert">${html(state.error)}</p><button type="button" class="admin-ai__button" data-action="admin-ai-load">Повторить</button></div>` : '<div class="admin-ai__loading">Загрузка настроек…</div>';
	const disabled = state.busy ? "disabled" : "";
	const models = state.models || [];
	return `<div class="admin-ai">
		<label class="admin-ai__power admin-toggle"><span>ИИ в поддержке</span><input type="checkbox" role="switch" aria-label="ИИ в поддержке" data-input="admin-ai-enabled" ${draft.enabled ? "checked" : ""} ${disabled}><i aria-hidden="true"></i></label>
		<section class="admin-ai__card"><div class="admin-ai__card-title"><span>01</span><h3>Подключение</h3><small class="${state.verified ? "is-ready" : ""}">${state.verified ? "Проверено" : "API"}</small></div>
			<label class="admin-field"><span>URL сервера</span><input class="admin-field__control" type="url" data-input="admin-ai-apiUrl" value="${attr(draft.apiUrl || "")}" placeholder="https://ваш-сервер/v1" autocomplete="off" ${disabled}></label>
			<p class="admin-ai__hint">Введите API URL вашего провайдера. К адресу без пути автоматически добавится /v1.</p>
			<label class="admin-field"><span>API-ключ</span><input class="admin-field__control" type="password" data-input="admin-ai-apiKey" value="${attr(draft.apiKey || "")}" placeholder="${draft.keyConfigured ? "Ключ сохранён · введите новый для замены" : "sk_live… или ключ другого провайдера"}" autocomplete="new-password" spellcheck="false" ${disabled}></label>
			<button class="admin-ai__button admin-ai__button--secondary" type="button" data-action="admin-ai-check" ${disabled}>${icon("check")}<span>${state.busy === "check" ? "Проверяем…" : "Проверить и загрузить модели"}</span></button>
			${state.error ? `<p class="admin-ai__error" role="alert">${html(state.error)}</p>` : ""}
		</section>
		<section class="admin-ai__card"><div class="admin-ai__card-title"><span>02</span><h3>Модель</h3>${models.length ? `<small>${models.length} доступно</small>` : ""}</div>
			${models.length ? `<div class="admin-ai__models" role="radiogroup" aria-label="Модель ИИ">${models.map(model => `<button type="button" role="radio" aria-checked="${draft.model === model}" class="admin-ai__model ${draft.model === model ? "is-selected" : ""}" data-action="admin-ai-model" data-value="${attr(model)}" ${disabled}><span>${html(model)}</span><i>${draft.model === model ? icon("check") : ""}</i></button>`).join("")}</div>` : `<p class="admin-ai__hint">${draft.model ? `Выбрана ${html(draft.model)}. Проверка подключения обновит список.` : "Проверьте подключение, чтобы выбрать доступную модель."}</p>`}
		</section>
		<section class="admin-ai__card"><div class="admin-ai__card-title"><span>03</span><h3>Поведение</h3><button type="button" class="admin-ai__reset" data-action="admin-ai-reset" ${disabled}>Базовый промпт</button></div>
			<label class="admin-field"><span>Ответов ИИ до кнопки «Позвать человека»</span><input class="admin-field__control" type="number" min="1" max="20" data-input="admin-ai-handoffAfter" value="${attr(draft.handoffAfter || 4)}" ${disabled}></label>
			<label class="admin-field"><span>Системный промпт</span><textarea class="admin-field__control admin-ai__prompt" data-input="admin-ai-prompt" rows="12" maxlength="16000" placeholder="Имя помощника, стиль общения, инструкции и правила передачи оператору" ${disabled}>${html(draft.prompt || "")}</textarea></label>
			<p class="admin-ai__hint">Здесь задаются имя, представление, тон и инструкции помощника. Изменение баланса и подписок выполняет администратор.</p>
		</section>
		<button type="button" class="admin-ai__button" data-action="admin-ai-save" ${disabled}>${icon("check")}<span>${state.busy === "save" ? "Сохраняем…" : "Сохранить настройки"}</span></button>
	</div>`;
}

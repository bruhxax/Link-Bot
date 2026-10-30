import {renderTelegramMessagePreview} from './telegram-message-preview.mjs';
export const escapeTorrent = value => String(value??'').replace(/[&<>"']/g,c=>({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;',"'":'&#39;'}[c]));
const svg = paths => `<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.7" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">${paths}</svg>`;
const icons={refresh:svg('<path d="M20 11a8 8 0 0 0-14.9-4M4 4v5h5M4 13a8 8 0 0 0 14.9 4M20 20v-5h-5"/>'),chevron:svg('<path d="m6 9 6 6 6-6"/>'),search:svg('<circle cx="10" cy="10" r="6"/><path d="m15 15 5 5"/>')};
export const torrentFilters=[['all','Все отчёты'],['active','Заблокированы'],['released','Срок истёк'],['observed','Без блокировки']];
export const torrentVariables={node:'Нода',duration:'Срок блокировки',unblock_at:'Время снятия',time:'Время события',username:'Пользователь',status:'Результат',ip:'IP пользователя',user_uuid:'UUID пользователя'};
export const torrentSample={node:'Германия · Frankfurt',duration:'1 ч.',unblock_at:'29.10.2026 01:40 МСК',time:'29.10.2026 00:40 МСК',username:'alex_vpn',status:'IP заблокирован на ноде',ip:'203.0.113.42',user_uuid:'4c7b531d-0c18-4af2-b14a-4d8769f17ab2'};
export function torrentTemplatePreview(text,user=false,render=renderTelegramMessagePreview){
 const replaced=String(text||'').replace(/\{([a-zA-Z_]+)\}/g,(token,key)=>user&&['ip','user_uuid'].includes(key)?token:Object.hasOwn(torrentSample,key)?escapeTorrent(torrentSample[key]):token);
 return render(replaced,true,escapeTorrent);
}
export function torrentDuration(seconds){const s=Math.max(0,Number(seconds)||0);return s&&s%86400===0?`${s/86400} дн.`:s&&s%3600===0?`${s/3600} ч.`:s&&s%60===0?`${s/60} мин.`:`${s} сек.`;}
export function torrentReportStatus(item,now=Date.now()){return !item.blocked?'observed':item.unblockAt&&Date.parse(item.unblockAt)>now?'active':'released';}
const date=value=>value?new Date(value).toLocaleString('ru-RU',{day:'2-digit',month:'short',hour:'2-digit',minute:'2-digit'}):'—';

export function createTorrentController({post,render,isActive=()=>true}){
 const model={data:null,query:'',status:'all',tab:'reports',busy:false,error:'',loaded:false};let latest=0;
 async function load({more=false,silent=false}={}){
  if(more&&(model.busy||!model.data?.hasMore))return;
  const request=++latest,query=model.query,status=model.status;
  model.busy=true;model.error='';if(!silent&&isActive())render();
  try{const response=await post('/api/mini-app/admin/torrents/reports',{query,status,beforeId:more?model.data.items.at(-1)?.id||0:0});
   if(request!==latest||query!==model.query||status!==model.status||!isActive())return;
   const incoming=response.data||{items:[]};const items=more?[...(model.data?.items||[]),...incoming.items]:incoming.items;
   model.data={...incoming,items:[...new Map(items.map(item=>[item.id,item])).values()]};model.loaded=true;
  }catch(error){if(request===latest&&query===model.query&&status===model.status&&isActive())model.error=error.message||'Не удалось загрузить отчёты';}
  finally{if(request===latest){model.busy=false;if(isActive())render();}}
 }
 return {model,load};
}

function reportCard(item,canUsers){
 const status=torrentReportStatus(item),labels={active:'IP заблокирован',released:'Срок блокировки истёк',observed:'Без блокировки'};
 return `<article class="tp-report" data-report-status="${status}"><header><div class="tp-report__identity"><strong>${escapeTorrent(item.username||'Пользователь панели')}</strong><code>${escapeTorrent(item.userUuid)}</code></div><span class="tp-badge tp-badge--${status}">${labels[status]}</span></header>
 <div class="tp-report__node"><span>${escapeTorrent(item.node||'Нода без названия')}</span></div><div class="tp-report__facts"><div><small>IP-адрес</small><code>${escapeTorrent(item.ip)}</code></div><div><small>Срок блокировки</small><strong>${item.blocked?torrentDuration(item.duration):'Не применялась'}</strong></div></div>
 <div class="tp-report__until"><span>${item.blocked?`Снятие блокировки <strong>${escapeTorrent(date(item.unblockAt))}</strong>`:'Плагин сообщил о трафике без блокировки IP'}</span></div>
 <footer><span>${escapeTorrent(date(item.occurredAt))}${item.network?` <i>·</i> ${escapeTorrent(item.network.toUpperCase())}`:''}</span>${canUsers&&item.customerId?`<button type="button" data-action="torrent-open-user" data-value="${Number(item.customerId)}">Открыть пользователя ↗</button>`:'<span class="tp-muted">Аккаунт не привязан</span>'}</footer></article>`;
}
function templateEditor(settings,kind,preview){
 const user=kind==='user',title=user?'Пользователю':'Администратору',text=settings[`${kind}Text`]||'',enabled=settings[`${kind}Enabled`]!==false;
 const variables=Object.entries(torrentVariables).filter(([key])=>!user||!['ip','user_uuid'].includes(key));
 return `<article class="tp-template" data-template="${kind}"><header><div><h3>${title}</h3><p>${user?'Нода и срок блокировки':'Детали события и IP пользователя'}</p></div><label class="tp-switch"><input type="checkbox" aria-label="Уведомлять ${user?'пользователя':'администратора'}" data-setting-path="torrents.${kind}Enabled" data-setting-type="boolean" ${enabled?'checked':''}><span></span></label></header>
 <label class="tp-template__label" for="torrent-${kind}-text">Текст сообщения <span>Telegram HTML</span></label><textarea id="torrent-${kind}-text" data-input="torrent-template" data-kind="${kind}" maxlength="2000" rows="7" spellcheck="false">${escapeTorrent(text)}</textarea>
 <div class="tp-variables" aria-label="Переменные сообщения">${variables.map(([key,label])=>`<button type="button" title="${escapeTorrent(label)}" data-action="torrent-variable" data-kind="${kind}" data-value="${key}">{${key}}</button>`).join('')}</div>
 <div class="tp-preview-head"><span>ПРЕДПРОСМОТР</span><small>Пример сообщения</small></div><div class="tp-telegram"><div class="tp-preview" data-torrent-preview="${kind}">${torrentTemplatePreview(text,user,preview)}</div><span class="tp-telegram__stamp">00:40 ✓✓</span></div></article>`;
}
export function renderTorrentWorkspace(model,settings={},options={}){
 const data=model.data,summary=data?.summary||{},isTemplates=model.tab==='templates';
 return `<div class="torrent-workspace">
 <div class="tp-metrics">${[['total','Всего отчётов','Все полученные события Torrent Blocker'],['active','Активных блокировок','Блокировки, срок которых ещё не истёк'],['today','За 24 часа','Отчёты за последние 24 часа'],['users','Пользователей','Уникальные пользователи в отчётах']].map(([key,label,hint])=>`<div class="tp-metric" data-metric="${key}" title="${hint}" aria-label="${hint}"><span>${label}</span><strong>${data?Number(summary[key]||0).toLocaleString('ru-RU'):'—'}</strong></div>`).join('')}</div>
 <details class="tp-setup"><summary><span class="tp-setup__heading"><strong>Подключение к Remnawave</strong><small>Источник отчётов и правила блокировки</small></span><span class="tp-connection ${data?.webhookConfigured?'is-ready':''}"><i></i>${data?.webhookConfigured?'Webhook настроен':'Требуется настройка'}</span><span class="tp-setup__chevron" aria-hidden="true">${options.chevronIcon||icons.chevron}</span></summary><div class="tp-setup__body"><p>В панели включите событие <code>torrent_blocker.report</code> для webhook и укажите адрес бота с путём <code>/api/remnawave/webhook</code>. Секрет <code>REMNAWAVE_WEBHOOK_SECRET</code> в боте должен совпадать с <code>WEBHOOK_SECRET_HEADER</code> панели.</p><p>Отчёты поступают из Torrent Blocker. Длительность блокировки настраивается в плагине.</p></div></details>
 <nav class="tp-tabs" aria-label="Разделы торрентов">${[['reports','Отчёты','События и блокировки'],['templates','Уведомления','Тексты сообщений']].map(([key,label,hint])=>`<button type="button" data-action="torrent-tab" data-value="${key}" aria-pressed="${model.tab===key}" title="${hint}"><strong>${label}</strong><small>${hint}</small></button>`).join('')}</nav>
 ${isTemplates?`<div class="tp-templates">${templateEditor(settings,'user',options.preview)}${templateEditor(settings,'admin',options.preview)}</div><p class="tp-autosave" data-torrent-save-status data-state="${escapeTorrent(options.saveStatus||'saved')}" role="status">${escapeTorrent(options.saveMessage||'Изменения сохраняются автоматически')}</p>`:
 `<div class="tp-toolbar"><div class="tp-search">${icons.search}<input type="search" aria-label="Поиск торрент-отчётов" placeholder="Пользователь, IP или нода" maxlength="160" data-input="torrent-search" value="${escapeTorrent(model.query)}"><button type="button" data-action="torrent-search">Найти</button></div><button class="tp-refresh" type="button" data-action="torrent-refresh" aria-label="Обновить отчёты" ${model.busy?'disabled':''}>${options.refreshIcon||icons.refresh}</button></div><div class="tp-filters">${torrentFilters.map(([key,label])=>`<button type="button" data-action="torrent-filter" data-value="${key}" aria-pressed="${model.status===key}">${label}</button>`).join('')}</div>
 ${model.error?`<div class="tp-error" role="alert">${escapeTorrent(model.error)} <button type="button" data-action="torrent-refresh">Повторить</button></div>`:''}
 <div class="tp-reports" aria-busy="${model.busy}">${data?.items?.length?data.items.map(item=>reportCard(item,options.canUsers)).join(''):model.busy?'<div class="tp-empty"><span class="tp-loader"></span><h3>Загружаем отчёты</h3><p>Получаем последние события блокировщика</p></div>':`<div class="tp-empty"><h3>${model.query||model.status!=='all'?'Отчёты не найдены':'Здесь появятся торрент-отчёты'}</h3><p>${model.query||model.status!=='all'?'Попробуйте другой запрос или фильтр':'Включите Torrent Blocker на нодах и доставку его событий в бот.'}</p></div>`}</div>${data?.hasMore?`<button class="tp-more" type="button" data-action="torrent-more" ${model.busy?'disabled':''}>${model.busy?'Загрузка…':'Показать ещё'}</button>`:''}`}</div>`;
}

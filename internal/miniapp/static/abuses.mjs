const esc=value=>String(value??'').replace(/[&<>"']/g,ch=>({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;',"'":'&#39;'}[ch]));
const icon=(name)=>({shield:'<path d="M12 2 20 6v6c0 5-3.5 8-8 10-4.5-2-8-5-8-10V6l8-4Z"/><path d="M9 12h6"/>',devices:'<rect x="3" y="4" width="18" height="13" rx="2"/><path d="M8 21h8M12 17v4"/>',network:'<circle cx="5" cy="5" r="2"/><circle cx="19" cy="5" r="2"/><circle cx="12" cy="19" r="2"/><path d="m6 7 5 10M18 7l-5 10M7 5h10"/>',refresh:'<path d="M20 7v5h-5M4 17v-5h5M5 7a8 8 0 0 1 13-2l2 2M4 17l2 2a8 8 0 0 0 13-2"/>',chevron:'<path d="m6 9 6 6 6-6"/>',clock:'<circle cx="12" cy="12" r="9"/><path d="M12 7v5l3 2"/>',search:'<circle cx="10" cy="10" r="6"/><path d="m15 15 5 5"/>',lock:'<rect x="5" y="10" width="14" height="11" rx="2"/><path d="M8 10V7a4 4 0 0 1 8 0v3"/>',revoke:'<path d="M4 7h16M7 7V4h10v3m1 0-1 13H7L6 7M10 11v5M14 11v5"/>'}[name]||'');
const svg=name=>`<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.7" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">${icon(name)}</svg>`;
const date=value=>value?new Date(value).toLocaleString('ru-RU',{day:'2-digit',month:'short',year:'numeric',hour:'2-digit',minute:'2-digit'}):'—';

export function createAbuseController({post,render,isActive=()=>true}){
 const model={data:null,busy:false,error:'',filter:'all',query:'',open:new Set(),confirm:null,acting:'',loaded:false};let serial=0,timer;
 async function load({refresh=false,silent=false}={}){
  const request=++serial;clearTimeout(timer);model.busy=true;model.error='';if(!silent&&isActive())render();
  try{const response=await post('/api/mini-app/admin/abuses/scan',{refresh});if(request!==serial||!isActive())return;model.data=response.data;model.loaded=true;
   if(model.data?.scanning)timer=setTimeout(()=>{if(isActive())void load({silent:true});},2200);
  }catch(error){if(request===serial&&isActive())model.error=error.message||'Не удалось проверить подписки';}
  finally{if(request===serial){model.busy=false;if(isActive())render();}}
 }
 function toggle(key){model.open.has(key)?model.open.delete(key):model.open.add(key);render();}
 async function act(key,action){
  const item=model.data?.items?.find(row=>row.key===key);if(!item||model.acting)return;
  if(model.confirm?.key!==key||model.confirm?.action!==action){model.confirm={key,action};render();return;}
  model.acting=key;model.error='';render();
  try{
   if(action==='block')await post('/api/mini-app/admin/users/block',{customerId:item.customerId,blocked:true,reason:item.kind==='trial'?'Повторное использование HWID в пробных подписках':'IP-адресов больше лимита устройств'});
   else if(action==='revoke')await post('/api/mini-app/admin/users/subscription/delete',{customerId:item.customerId,subscriptionId:item.subscriptionId});
   else if(action==='latest'){
    const sub=item.subscriptions?.at(-1);if(!sub)throw Error('Подписка не найдена');
    await post('/api/mini-app/admin/users/subscription/delete',{customerId:sub.customerId,subscriptionId:sub.subscriptionId});
   }else if(action==='all'){
    for(const sub of item.subscriptions||[])await post('/api/mini-app/admin/users/subscription/delete',{customerId:sub.customerId,subscriptionId:sub.subscriptionId});
   }
   model.confirm=null;await load({refresh:true});
  }catch(error){model.error=error.message||'Действие не выполнено';}
  finally{model.acting='';render();}
 }
 return {model,load,toggle,act,cancel(){model.confirm=null;render();}};
}

function card(item,model,options){
 const trial=item.kind==='trial',open=model.open.has(item.key),confirm=model.confirm?.key===item.key;
 const title=trial?'Повторный триал':'Возможный шаринг';
 const number=trial?item.subscriptions?.length||0:item.ips?.length||0;
 const unit=trial?'пробных подписок':'IP-адресов';
 const actions=trial?[["block","Заблокировать","lock",options.canBlock],["latest","Отозвать последнюю подписку","revoke",options.canRevoke],["all","Отозвать все подписки","revoke",options.canRevoke]]:[["block","Заблокировать","lock",options.canBlock],["revoke","Отозвать подписку","revoke",options.canRevoke]];
 return `<article class="ab-card ${open?'is-open':''}" data-kind="${trial?'trial':'sharing'}"><button class="ab-card__head" type="button" data-action="abuse-toggle" data-value="${esc(item.key)}" aria-expanded="${open}"><span class="ab-card__glyph">${svg(trial?'devices':'network')}</span><span class="ab-card__main"><strong>${title}</strong><small>${esc(item.username||'Пользователь')} · ${number} ${unit}</small></span><span class="ab-card__count">${number}</span><span class="ab-card__chevron">${svg('chevron')}</span></button>
 <div class="ab-card__expand"><div class="ab-card__inside"><div class="ab-card__hint">${trial?'Один HWID найден на нескольких пробных подписках':'Уникальных IP больше разрешённого числа устройств'}</div>
 ${trial?`<div class="ab-fact"><span>HWID устройства</span><code>${esc(item.hwid)}</code></div><div class="ab-list-label">Совпадающие пробные подписки</div><div class="ab-evidence">${(item.subscriptions||[]).map((sub,index)=>`<div class="ab-evidence__row"><span class="ab-evidence__mark">${index+1}</span><span><strong>${esc(sub.name||sub.username)}</strong><small>${esc(sub.username)} · ${date(sub.createdAt)}</small></span><code>#${Number(sub.subscriptionId)}</code></div>`).join('')}</div>`:
 `<div class="ab-fact"><span>Лимит устройств</span><strong>${Number(item.deviceLimit)}</strong></div><div class="ab-list-label">Все IP-адреса</div><div class="ab-evidence">${(item.ips||[]).map((ip,index)=>`<div class="ab-evidence__row"><span class="ab-evidence__mark">${index+1}</span><span><strong class="ab-mono">${esc(ip.ip)}</strong><small>${esc(ip.node||'Нода не указана')} · ${date(ip.lastSeen)}</small></span></div>`).join('')}</div>`}
 <div class="ab-card__actions">${actions.filter(([, , ,allowed])=>allowed).map(([action,label,glyph])=>`<button type="button" data-action="abuse-act" data-value="${esc(item.key)}" data-abuse-operation="${action}" class="${action==='block'?'ab-danger':''}" ${model.acting?'disabled':''}>${svg(glyph)}<span>${label}</span></button>`).join('')}</div>
 ${confirm?`<div class="ab-confirm" role="group"><strong>${model.confirm.action==='block'?'Заблокировать пользователя?':model.confirm.action==='all'?'Отозвать все совпадающие подписки?':'Отозвать подписку?'}</strong><span>Доступ будет отключён в панели Remnawave.</span><div><button type="button" data-action="abuse-cancel">Отмена</button><button type="button" data-action="abuse-act" data-value="${esc(item.key)}" data-abuse-operation="${esc(model.confirm.action)}" class="ab-confirm__yes" ${model.acting?'disabled':''}>${model.acting?'Выполняется…':'Подтвердить'}</button></div></div>`:''}</div></div></article>`;
}

export function renderAbuseWorkspace(model,options={}){
 const data=model.data||{},items=data.items||[],sharing=items.filter(item=>item.kind==='sharing').length,trial=items.filter(item=>item.kind==='trial').length;
 const query=model.query.trim().toLowerCase(),visible=items.filter(item=>(model.filter==='all'||model.filter===item.kind)&&(!query||[item.username,item.hwid,...(item.ips||[]).map(ip=>ip.ip),...(item.subscriptions||[]).map(sub=>sub.username)].some(value=>String(value||'').toLowerCase().includes(query))));
 return `<div class="ab-workspace"><header class="ab-hero"><div>${options.showTitle?'<span class="ab-overline">БЕЗОПАСНОСТЬ ПОДПИСОК</span><h1>Абузы</h1>':''}<p>Подозрительные подключения и повторные пробные периоды</p></div><button type="button" class="ab-refresh" data-action="abuse-refresh" ${data.scanning?'disabled':''}>${svg('refresh')}<span>${data.scanning?'Проверяем…':'Проверить сейчас'}</span></button></header>
 <div class="ab-metrics"><div><span>Всего случаев</span><strong>${items.length}</strong><small>Требуют проверки</small>${svg('shield')}</div><div><span>Возможный шаринг</span><strong>${sharing}</strong><small>IP выше лимита</small>${svg('network')}</div><div><span>Повторный триал</span><strong>${trial}</strong><small>Совпадения HWID</small>${svg('devices')}</div></div>
 <div class="ab-toolbar"><div class="ab-search">${svg('search')}<input type="search" data-input="abuse-search" aria-label="Поиск абузов" placeholder="Пользователь, IP или HWID" value="${esc(model.query)}"></div><div class="ab-tabs">${[['all','Все'],['sharing','Шаринг'],['trial','Триалы']].map(([key,label])=>`<button type="button" data-action="abuse-filter" data-value="${key}" aria-pressed="${model.filter===key}">${label}</button>`).join('')}</div></div>
 ${data.scanning?`<div class="ab-status is-loading"><span class="ab-pulse"></span><span>Проверяем подключения и устройства в Remnawave…</span></div>`:''}
 ${data.error?`<div class="ab-status is-error">${esc(data.error)}</div>`:''}${data.failedChecks?`<div class="ab-status is-warning">Не удалось проверить ${Number(data.failedChecks)} источников данных. Список может быть неполным.</div>`:''}${model.error?`<div class="ab-status is-error" role="alert">${esc(model.error)}</div>`:''}
 <div class="ab-cards">${visible.length?visible.map(item=>card(item,model,options)).join(''):`<div class="ab-empty">${svg('shield')}<h2>${data.scanning?'Проверка выполняется':query||model.filter!=='all'?'Ничего не найдено':data.checkedAt?'Подозрительных случаев нет':'Начинаем проверку'}</h2><p>${data.scanning?'Результаты появятся после ответа панели':query||model.filter!=='all'?'Измените поиск или фильтр':data.checkedAt?'По текущим данным совпадений не обнаружено':'Получаем данные Remnawave'}</p></div>`}</div>
 <footer class="ab-footer">${svg('clock')}<span>${data.checkedAt?`Проверено ${date(data.checkedAt)} · ${Number(data.checkedUsers||0)} подписок`:'Ожидаем первую проверку'}</span></footer></div>`;
}

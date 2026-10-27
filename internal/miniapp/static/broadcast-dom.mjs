// Update the broadcast editor without replacing connected controls. This keeps
// label activation, pointer presses, focus and CSS transitions intact during saves.
function identity(node) {
  if (node.nodeType !== 1) return `${node.nodeType}`;
  const d=node.dataset;
  const field=node.matches('label,div')&&node.querySelector(':scope > select');
  if(field)return `select-field:${identity(field)}`;
  if(d.revealContainer)return `container:${d.revealContainer}`;
  if(d.revealKey)return `reveal:${d.revealKey}`;
  for(const key of ['input','autoReplyField','audienceField','surveyField','broadcastField'])if(d[key])return `${node.tagName}:${key}:${d[key]}:${d.channel||''}:${d.question||''}:${d.option||''}:${d.broadcastIndex||''}:${node.type==='checkbox'?node.value:''}`;
  if(d.action)return `action:${d.action}:${d.value||''}:${d.question||''}:${d.option||''}`;
  if(node.id)return `id:${node.id}`;
  return `${node.tagName}:${node.getAttribute('class')||''}`;
}
function attributes(current,next) {
  for(const attr of [...current.attributes])if(!(current.tagName==='DETAILS'&&attr.name==='open')&&!next.hasAttribute(attr.name))current.removeAttribute(attr.name);
  for(const attr of next.attributes)if(!(current.tagName==='DETAILS'&&attr.name==='open')&&current.getAttribute(attr.name)!==attr.value)current.setAttribute(attr.name,attr.value);
}
function patch(current,next) {
  if(current.nodeType!==1){if(current.nodeValue!==next.nodeValue)current.nodeValue=next.nodeValue;return;}
  // The enhanced select owns a trigger and caption IDs; retain that mounted field.
  const select=current.querySelector(':scope > select[hidden]'),incoming=next.querySelector(':scope > select');
  if(select&&incoming){
    select.innerHTML=incoming.innerHTML;select.value=incoming.value;select.disabled=incoming.disabled;
    const trigger=current.querySelector('.admin-localization-select');
    if(trigger){trigger.textContent=select.selectedOptions[0]?.textContent||'';trigger.disabled=select.disabled;}
    const caption=current.querySelector(':scope > span'),nextCaption=next.querySelector(':scope > span');
    if(caption&&nextCaption)caption.textContent=nextCaption.textContent;
    return;
  }
  const open=current.tagName==='DETAILS'?current.open:null;
  attributes(current,next);
  if(current.tagName==='INPUT'){
    current.checked=next.checked;current.disabled=next.disabled;
    if(current.value!==next.value)current.value=next.value;
    return;
  }
  if(current.tagName==='TEXTAREA'){
    current.disabled=next.disabled;if(current.value!==next.value)current.value=next.value;return;
  }
  if(current.tagName==='SELECT'){
    if(current.innerHTML!==next.innerHTML)current.innerHTML=next.innerHTML;
    current.value=next.value;current.disabled=next.disabled;return;
  }
  const old=[...current.childNodes],used=new Set();let cursor=current.firstChild;
  for(const wanted of [...next.childNodes]){
    const key=identity(wanted),found=old.find(node=>!used.has(node)&&identity(node)===key);
    if(found){
      used.add(found);if(found!==cursor)current.insertBefore(found,cursor);patch(found,wanted);cursor=found.nextSibling;
    }else{current.insertBefore(wanted,cursor);}
  }
  for(const node of old)if(!used.has(node))node.remove();
  if(open!==null)current.open=open;
}
export function patchBroadcastWorkflow(current,next) {
  patch(current,next);
  return current;
}

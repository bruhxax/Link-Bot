// Telegram HTML is not browser HTML. Rebuild its small allowed subset, retaining
// text and safe links while dropping every event handler, style and active node.
export function renderTelegramMessagePreview(value, formatted, escapeHtml) {
  if(!formatted&&!/<\/?(?:b|strong|i|em|u|ins|s|strike|del|a|code|pre|blockquote|tg-spoiler|tg-emoji|span)(?:\s|>)/i.test(value||''))return escapeHtml(value||'');
  const template=document.createElement('template');template.innerHTML=String(value||'');
  const render=node=>{
    if(node.nodeType===3)return escapeHtml(node.textContent||'');
    if(node.nodeType!==1)return '';
    const children=[...node.childNodes].map(render).join(''),tag=node.tagName.toLowerCase();
    if(['b','strong','i','em','u','ins','s','strike','del','code','pre','blockquote'].includes(tag))return `<${tag}>${children}</${tag}>`;
    if(tag==='br')return '\n';
    if(tag==='a'){
      const href=node.getAttribute('href')||'';
      if(/^(https?:|tg:|mailto:)/i.test(href))return `<a href="${escapeHtml(href)}" target="_blank" rel="noopener noreferrer">${children}</a>`;
    }
    if(tag==='tg-spoiler'||tag==='span'&&node.classList.contains('tg-spoiler'))return `<span class="telegram-preview-spoiler">${children}</span>`;
    if(['script','style','iframe','object','svg','math'].includes(tag))return '';
    return children;
  };
  return [...template.content.childNodes].map(render).join('');
}

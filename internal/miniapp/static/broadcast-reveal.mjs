const records = new WeakMap();
const mounted = new WeakSet();
const timing = { duration: 260, easing: 'cubic-bezier(.22, 1, .36, 1)' };
function visible(node) {
  if (!node.getClientRects().length || !node.getBoundingClientRect().height) return false;
  for (let parent=node.parentElement;parent;parent=parent.parentElement) {
    if (parent.tagName==='DETAILS'&&!parent.open&&!parent.querySelector(':scope > summary')?.contains(node)) return false;
  }
  return true;
}
export function captureBroadcastReveals(root) {
  const workflow=root?.matches('.broadcast-workflow')?root:root?.querySelector('.broadcast-workflow');
  const keys=new Map(),containers=new Map();
  if (!workflow) return {scope:'',keys,containers};
  for(const node of workflow.querySelectorAll('[data-reveal-key]'))if(visible(node))keys.set(node.dataset.revealKey,node);
  for(const node of workflow.querySelectorAll('[data-reveal-container]'))if(visible(node)){
    const rect=node.getBoundingClientRect();
    containers.set(node.dataset.revealContainer,{node,height:rect.height,width:rect.width,keys:[...keys].filter(([,child])=>node.contains(child)).map(([key])=>key).sort().join('|')});
  }
  return {scope:workflow.dataset.revealScope||'',keys,containers};
}
export function playBroadcastReveals(root, previous = records.get(root)) {
  const next=captureBroadcastReveals(root);
  records.set(root,next);
  if(!mounted.has(root)){
    mounted.add(root);
    root.addEventListener('toggle',event=>{if(event.target.closest?.('.broadcast-workflow'))playBroadcastReveals(root);},true);
  }
  if(!previous||previous.scope!==next.scope||document.hidden||matchMedia('(prefers-reduced-motion: reduce)').matches)return next;
  const added=[...next.keys].filter(([key])=>!previous.keys.has(key)).map(([,node])=>node);
  for(const node of added)if(!added.some(parent=>parent!==node&&parent.contains(node))){
    node.animate([{opacity:0,transform:'translateY(6px)'},{opacity:1,transform:'translateY(0)'}],timing);
  }
  const resized=[...next.containers].flatMap(([key,current])=>{
    const old=previous.containers.get(key);
    return old&&old.keys!==current.keys&&Math.abs(old.width-current.width)<3&&Math.abs(old.height-current.height)>2?[{...current,from:old.height}]:[];
  });
  for(const current of resized)if(!resized.some(child=>child!==current&&current.node.contains(child.node))){
    const node=current.node,overflow=node.style.overflow;
    node.style.overflow='hidden';
    const animation=node.animate([{height:`${current.from}px`},{height:`${current.height}px`}],timing);
    animation.finished.catch(()=>{}).finally(()=>{node.style.overflow=overflow;});
  }
  return next;
}

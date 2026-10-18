import test from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs';
import vm from 'node:vm';

const source=fs.readFileSync(new URL('./static/app.js',import.meta.url),'utf8');
function load(ctx,name) {
 const start=source.indexOf(`function ${name}(`),end=source.indexOf('\nfunction ',start+1);
 assert.ok(start>=0&&end>start,name); vm.runInContext(source.slice(start,end),ctx);
}

test('rapid scroll restoration keeps the last editor position and cannot move another section',()=>{
 const frames=[],scroll={scrollTop:0};
 const state={currentPage:'admin',adminSection:'appearance',scrollTopByPage:{}};
 const ctx=vm.createContext({state,scrollRestoreGeneration:0,app:{querySelector:()=>scroll},requestAnimationFrame:callback=>frames.push(callback)});
 load(ctx,'restoreScrollPosition');
 ctx.restoreScrollPosition(700); assert.equal(scroll.scrollTop,700);
 ctx.restoreScrollPosition(820); assert.equal(scroll.scrollTop,820);
 frames.shift()(); assert.equal(scroll.scrollTop,820);
 frames.shift()(); assert.equal(scroll.scrollTop,820);
 ctx.restoreScrollPosition(900); state.adminSection='content'; scroll.scrollTop=25;
 frames.shift()(); assert.equal(scroll.scrollTop,25);
});

test('frames, glass, glow and feature switches update in place without replacing the page',()=>{
 const start=source.indexOf('const settingPath = target?.dataset?.settingPath;'),end=source.indexOf('const inputKey = target?.dataset?.input;',start);
 assert.ok(start>=0&&end>start);
 let renders=0,previews=0,saves=0;
 class Input {}
 const state={adminSettingsDraft:{},adminJSONDrafts:{}};
 const ctx=vm.createContext({state,HTMLInputElement:Input,render:()=>renders++,applyAppearance:()=>previews++,mountCustomBackgroundPreview(){},syncAdminSaveBarDOM:()=>saves++,app:{querySelector:()=>null},setDeepValue(object,path,value){const keys=path.split('.');for(const key of keys.slice(0,-1))object=object[key]||=( {} );object[keys.at(-1)]=value;}});
 vm.runInContext(`function updateSetting(target){${source.slice(start,end)}}`,ctx);
 for(const path of ['appearance.showFrames','appearance.glass','appearance.glow','features.reviews','content.startMenu.notificationsButton.enabled']) {
  const input=Object.assign(new Input(),{dataset:{settingPath:path,settingType:'boolean'},checked:true});
  ctx.updateSetting(input);
 }
 assert.equal(renders,0); assert.equal(previews,3); assert.ok(saves>=5); assert.equal(state.adminSettingsDirty,true);
});

test('editor focus follows the setting path when conditional fields change positions',()=>{
 const field=path=>({tagName:'INPUT',value:'on',disabled:false,matches:()=>true,getAttribute:name=>name==='data-setting-path'?path:'',focus(options){this.focusOptions=options;}});
 const original=field('appearance.glass');let controls=[field('appearance.glow'),original];
 const state={currentPage:'admin',adminSection:'appearance'};
 const ctx=vm.createContext({state,document:{activeElement:original},app:{contains:()=>true,querySelectorAll:()=>controls}});
 load(ctx,'captureRealtimeFocus');load(ctx,'restoreRealtimeFocus');const snapshot=ctx.captureRealtimeFocus();
 const replacement=field('appearance.glass');controls=[field('appearance.newField'),field('appearance.glow'),replacement];
 ctx.restoreRealtimeFocus(snapshot);
 assert.equal(replacement.focusOptions?.preventScroll,true); assert.equal(controls[1].focusOptions,undefined);
});

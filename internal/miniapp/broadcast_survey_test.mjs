import test from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs';
import vm from 'node:vm';
const source=fs.readFileSync(new URL('./static/app.js',import.meta.url),'utf8');
function load(c,name){let start=source.indexOf(`function ${name}(`);assert.ok(start>=0,name);if(source.slice(start-6,start)==='async ')start-=6;const next=source.slice(start+1).search(/\n(?:async )?function /);vm.runInContext(source.slice(start,start+1+next),c);}
function harness(){
 const requests=[],renders=[],state={adminAudienceDraft:{},adminBroadcast:{status:'draft',updatedAt:'stamp'},adminBroadcastButtonsDraft:[]};
 const escape=s=>String(s).replace(/[&<>"']/g,c=>({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;',"'":'&#39;'}[c]));
 const c=vm.createContext({state,canAdmin:()=>true,deepClone:x=>JSON.parse(JSON.stringify(x)),render:x=>renders.push(x),icon:()=>'<svg></svg>',escapeHtml:escape,escapeAttribute:escape,post:async(url,body)=>{requests.push({url,body});return{data:{status:'draft',updatedAt:'new-stamp',survey:JSON.parse(JSON.stringify(body.survey))}};}});
 for(const name of ['defaultAdminSurvey','updateAdminSurveyField','editAdminSurvey','saveAdminSurvey','loadAdminSurveyResults','surveyPlainText','renderAdminSurvey','renderAdminSurveyResults','setAdminBroadcastDraft'])load(c,name);
 return{c,state,requests,renders};
}
test('survey editor can add questions and arbitrary answer buttons and keeps dirty input on polling',()=>{
 const{c,state}=harness();c.setAdminBroadcastDraft(state.adminBroadcast);
 c.updateAdminSurveyField({dataset:{surveyField:'enabled'},checked:true});
 assert.equal(state.adminSurveyDraft.questions.length,1);assert.equal(state.adminSurveyDirty,true);
 c.editAdminSurvey('addQuestion');c.editAdminSurvey('addOption',1);
 assert.equal(state.adminSurveyDraft.questions[1].options.length,3);
 c.updateAdminSurveyField({dataset:{surveyField:'option',question:'1',option:'2'},value:'Другое'});
 c.setAdminBroadcastDraft({status:'draft',updatedAt:'poll',survey:{enabled:false}});
 assert.equal(state.adminSurveyDraft.enabled,true);assert.equal(state.adminSurveyDraft.questions[1].options[2],'Другое');
 c.editAdminSurvey('removeOption',1,1);c.editAdminSurvey('removeQuestion',0);assert.equal(state.adminSurveyDraft.questions.length,1);
 const html=c.renderAdminSurvey(false);assert.match(html,/role="switch"/);assert.match(html,/survey-question-add/);assert.match(html,/survey-option-add/);
});
test('saving uses the current draft version and reset discards old survey edits',async()=>{
 const{c,state,requests}=harness();c.updateAdminSurveyField({dataset:{surveyField:'enabled'},checked:true});
 await c.saveAdminSurvey();assert.equal(requests[0].url,'/api/mini-app/admin/broadcast/survey');assert.equal(requests[0].body.updatedAt,'stamp');assert.equal(state.adminSurveyDirty,false);
 c.updateAdminSurveyField({dataset:{surveyField:'title'},value:'Unsaved'});c.setAdminBroadcastDraft({status:'idle',survey:{}},{forceButtons:true});assert.equal(state.adminSurveyDraft.title,undefined);assert.equal(state.adminSurveyDirty,false);
});
test('running surveys cannot be modified and all controls are disabled',()=>{
 const{c,state}=harness();state.adminSurveyDraft=c.defaultAdminSurvey();state.adminSurveyDraft.enabled=true;state.adminBroadcast.status='running';
 c.editAdminSurvey('addQuestion');c.updateAdminSurveyField({dataset:{surveyField:'title'},value:'bad'});assert.equal(state.adminSurveyDraft.questions.length,1);assert.notEqual(state.adminSurveyDraft.title,'bad');
 assert.match(c.renderAdminSurvey(true),/data-survey-field="title"[^>]*disabled/);
});
test('results escape customer content and expose counts, individual answers and pagination',()=>{
 const{c,state}=harness();state.adminSurveyResultsOpen=true;state.adminSurveyResults={runs:[],report:{run:{id:'run',config:{questions:[{text:'<b>Service?</b>',options:['<script>evil</script>','Good']}]},delivered:3,started:2,completed:1},counts:[[0,1]],recipients:[{id:1,telegramId:42,username:'<img onerror=evil>',step:1,answers:[1]}],hasMore:true}};
 const html=c.renderAdminSurveyResults();assert.doesNotMatch(html,/<script>|<img /);assert.match(html,/&lt;script&gt;/);assert.match(html,/Good/);assert.match(html,/data-more="true"/);assert.match(html,/Доставлено/);
});
test('preview, launch and templates save survey before other pending broadcast settings',()=>{
 const start=source.indexOf('async function ensureAdminBroadcastButtonsSaved('),end=source.indexOf('\nasync function previewAdminBroadcast(',start),body=source.slice(start,end);
 assert.ok(body.indexOf('saveAdminSurvey()')<body.indexOf('saveAdminBroadcastExpiration'));
 const native=fs.readFileSync(new URL('../../admin-ui/broadcast.jsx',import.meta.url),'utf8');assert.match(native,/surveyHTML/);assert.match(native,/Кнопки в конце опроса/);
});

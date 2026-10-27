import test from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs';
import vm from 'node:vm';
const source=fs.readFileSync(new URL('./static/app.js',import.meta.url),'utf8');
function load(c,name){let start=source.indexOf(`function ${name}(`);assert.ok(start>=0,name);if(source.slice(start-6,start)==='async ')start-=6;const next=source.slice(start+1).search(/\n(?:async )?function /);vm.runInContext(source.slice(start,start+1+next),c);}
function harness(){
 const requests=[],renders=[],state={adminAudienceDraft:{},adminBroadcast:{status:'draft',updatedAt:'stamp'},adminBroadcastButtonsDraft:[]};
 const escape=s=>String(s).replace(/[&<>"']/g,c=>({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;',"'":'&#39;'}[c]));
 const c=vm.createContext({scheduleAdminBroadcastSave(){},flushAdminBroadcastAutoSave:async()=>{},discardAdminBroadcastAutoSave:async()=>{},renderTelegramMessagePreview:(text,formatted,escape)=>escape(text),state,setTimeout:()=>0,canAdmin:()=>true,deepClone:x=>JSON.parse(JSON.stringify(x)),render:x=>renders.push(x),icon:()=>'<svg></svg>',escapeHtml:escape,escapeAttribute:escape,post:async(url,body)=>{requests.push({url,body});return{data:{status:'draft',updatedAt:'new-stamp',survey:JSON.parse(JSON.stringify(body.survey))}};}});
 for(const name of ['defaultAdminSurvey','updateAdminSurveyField','editAdminSurvey','saveAdminSurvey','loadAdminSurveyResults','surveyPlainText','renderAdminSurveyMessage','renderAdminSurvey','renderAdminSurveyResults','setAdminBroadcastDraft','adminBroadcastWorkflowMode','activeAdminBroadcastButtons','markAdminBroadcastButtonsDirty','setAdminBroadcastWorkflowMode','adminBroadcastReadiness','renderAdminBroadcastWorkflowTabs','renderAdminBroadcastOptionalButtons','renderAdminBroadcastButton'])load(c,name);
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
 const html=c.renderAdminSurvey(false);assert.match(html,/Заменить в Telegram/);assert.doesNotMatch(html,/<textarea/);assert.match(html,/survey-question-add/);assert.match(html,/survey-option-add/);
});
test('saving uses the current draft version and reset discards old survey edits',async()=>{
 const{c,state,requests}=harness();c.updateAdminSurveyField({dataset:{surveyField:'enabled'},checked:true});
 await c.saveAdminSurvey();assert.equal(requests[0].url,'/api/mini-app/admin/broadcast/survey');assert.equal(requests[0].body.updatedAt,'stamp');assert.equal(state.adminSurveyDirty,false);
 c.updateAdminSurveyField({dataset:{surveyField:'title'},value:'Unsaved'});c.setAdminBroadcastDraft({status:'idle',survey:{}},{forceButtons:true,forceSurvey:true});assert.equal(state.adminSurveyDraft.title,undefined);assert.equal(state.adminSurveyDirty,false);
});
test('running surveys cannot be modified and all controls are disabled',()=>{
 const{c,state}=harness();state.adminSurveyDraft=c.defaultAdminSurvey();state.adminSurveyDraft.enabled=true;state.adminBroadcast.status='running';
 c.editAdminSurvey('addQuestion');c.updateAdminSurveyField({dataset:{surveyField:'title'},value:'bad'});assert.equal(state.adminSurveyDraft.questions.length,1);assert.notEqual(state.adminSurveyDraft.title,'bad');
 assert.match(c.renderAdminSurvey(true),/data-survey-field="startText"[^>]*disabled/);
});
test('results escape customer content and expose counts, individual answers and pagination',()=>{
 const{c,state}=harness();state.adminSurveyResultsOpen=true;state.adminSurveyResults={runs:[],report:{run:{id:'run',config:{questions:[{text:'<b>Service?</b>',options:['<script>evil</script>','Good']}]},delivered:3,started:2,completed:1},counts:[[0,1]],recipients:[{id:1,telegramId:42,username:'<img onerror=evil>',step:1,answers:[1]}],hasMore:true}};
 const html=c.renderAdminSurveyResults();assert.doesNotMatch(html,/<script>|<img /);assert.match(html,/&lt;script&gt;/);assert.match(html,/Good/);assert.match(html,/data-more="true"/);assert.match(html,/Доставлено/);
});
test('preview, launch and templates save survey before other pending broadcast settings',()=>{
 const start=source.indexOf('async function ensureAdminBroadcastButtonsSaved('),end=source.indexOf('\nasync function previewAdminBroadcast(',start),body=source.slice(start,end);
 assert.ok(body.indexOf('saveAdminSurvey()')<body.indexOf('saveAdminBroadcastExpiration'));
 const native=fs.readFileSync(new URL('../../admin-ui/broadcast.jsx',import.meta.url),'utf8');assert.match(native,/telegramWorkflowHTML/);
});

test('switching workflows preserves separate texts and buttons without requiring a captured survey invitation',async()=>{
 const{c,state}=harness();
 c.setAdminBroadcastDraft({status:'finished',sourceKind:'text',sourcePreview:'Обычная реклама',buttons:[{id:'ad',text:'Купить'}],survey:{enabled:false}});
 await c.setAdminBroadcastWorkflowMode('survey');
 assert.equal(c.activeAdminBroadcastButtons().length,0,'ordinary ad buttons must not become survey buttons');
 state.adminSurveyDraft.introText='Наше приглашение';state.adminSurveyDraft.questions=[{text:'Как сервис?',options:['Хорошо','Плохо']}];
 c.activeAdminBroadcastButtons().push({id:'survey',text:'Спасибо'});c.markAdminBroadcastButtonsDirty();
 assert.equal(c.adminBroadcastReadiness().ready,true);
 state.adminBroadcast.sourceKind='';assert.equal(c.adminBroadcastReadiness().ready,true,'survey must not need a captured source');
 await c.setAdminBroadcastWorkflowMode('message');
 assert.equal(c.activeAdminBroadcastButtons()[0].id,'ad');assert.equal(c.adminBroadcastReadiness().ready,false);
 await c.setAdminBroadcastWorkflowMode('survey');
 assert.equal(state.adminSurveyDraft.introText,'Наше приглашение');assert.equal(c.activeAdminBroadcastButtons()[0].id,'survey');
 c.setAdminBroadcastDraft({status:'draft',survey:{enabled:false},buttons:[]},{forceButtons:true});
 assert.equal(state.adminSurveyDraft.introText,'Наше приглашение','polling/button save discarded survey edits');
});

test('survey answers stay inside the survey composer while completed broadcast progress stays hidden',()=>{
 const{c,state}=harness();
 Object.assign(c,{renderAdminBroadcastTabs:()=>'',renderCurrentAudience:()=>'<div>Audience controls</div>',renderAdminBroadcastExpiration:()=>'<div>Timer control</div>',renderAdminBroadcastAutoReply:()=>'<div>Reply control</div>',pageClass:()=>''});
 load(c,'renderAdminBroadcastLiveProgress');load(c,'renderAdminBroadcastPage');
 state.adminBroadcast={status:'finished',sourceKind:'text',sourcePreview:'An ad',recipientCount:100,sentCount:99,failedCount:1,lastError:'blocked-old-error'};
 let html=c.renderAdminBroadcastPage(true);
 assert.doesNotMatch(html,/blocked-old-error|100%|admin-broadcast__progress|Проверьте предпросмотр|broadcast-extra|data-value="results"/);
 assert.match(html,/Отправить/);assert.match(html,/Timer control/);assert.match(html,/Reply control/);
 state.adminBroadcastMode='survey';state.adminSurveyDraft=c.defaultAdminSurvey();state.adminSurveyResultsOpen=true;state.adminSurveyResults={runs:[]};
 html=c.renderAdminBroadcastPage(true);
 assert.match(html,/Ответы на опросы/);assert.match(html,/Отправленных опросов пока нет/);
 assert.match(html,/Audience controls/);assert.match(html,/admin-broadcast-open-confirm/);assert.match(html,/survey-question-add/);
 assert.doesNotMatch(html,/data-value="results"|broadcast-step|Reply control/);
 assert.equal((html.match(/role="tab"/g)||[]).length,2);
});

test('an active campaign locks the other composer without adding a results category',async()=>{
 const{c,state}=harness();state.adminBroadcast={status:'running',survey:{enabled:true}};state.adminBroadcastMode='survey';
 await c.setAdminBroadcastWorkflowMode('message');assert.equal(state.adminBroadcastMode,'survey');
 const html=c.renderAdminBroadcastWorkflowTabs();assert.match(html,/data-value="message" disabled/);assert.doesNotMatch(html,/data-value="results"/);
});

test('questions can all collapse to leave a compact survey overview',()=>{
 const{c,state}=harness();state.adminSurveyDraft=c.defaultAdminSurvey();state.adminSurveyOpenQuestion=-1;
 const html=c.renderAdminSurvey(false);
 assert.match(html,/data-action="survey-question-open"[^>]*aria-expanded="false"/);
 assert.doesNotMatch(html,/class="survey-question__body/);
});

test('saving recipients or ordinary buttons cannot discard the saved survey before send confirmation',()=>{
 const{c,state}=harness();const survey={...c.defaultAdminSurvey(),enabled:true,introText:'Наш опрос',questions:[{text:'Вопрос?',options:['Да']}],finalButtons:[{id:'end',type:'support',text:'Поддержка'}]};
 c.setAdminBroadcastDraft({status:'draft',updatedAt:'survey-saved',buttons:[],survey});
 c.setAdminBroadcastDraft({status:'draft',updatedAt:'audience-saved',buttons:[],audience:{category:'paid'}});
 assert.equal(state.adminSurveyDraft.introText,'Наш опрос');assert.equal(state.adminBroadcast.survey.enabled,true);assert.equal(c.adminBroadcastReadiness().ready,true);assert.equal(c.activeAdminBroadcastButtons()[0].id,'end');
});

test('editing final buttons updates the survey only through the shared input handler',()=>{
 const{c,state}=harness();load(c,'updateConsoleBroadcastButton');
 state.adminBroadcastMode='survey';state.adminSurveyDraft={...c.defaultAdminSurvey(),finalButtons:[{text:'Old',type:'url',url:'https://example.org'}]};
 state.adminBroadcastButtonsDraft=[{text:'Buy',type:'url',url:'https://example.org/buy'}];
 c.updateConsoleBroadcastButton(0,'text','Поддержка');c.updateConsoleBroadcastButton(0,'type','support');
 assert.equal(state.adminSurveyDraft.finalButtons[0].text,'Поддержка');assert.equal(state.adminSurveyDraft.finalButtons[0].url,'');
 assert.equal(state.adminSurveyDirty,true);assert.equal(state.adminBroadcastButtonsDraft[0].text,'Buy');
 assert.ok(source.includes('updateConsoleBroadcastButton(broadcastIndex,broadcastField,target.value)'));
});

test('survey editor removes decorative numbers and repeated answer labels but keeps accessible option names',()=>{
 const{c,state}=harness();state.adminBroadcastMode='survey';state.adminSurveyDraft={...c.defaultAdminSurvey(),questions:[{text:'Выберите сервер',options:['Швейцария','Турция','США','Нидерланды','Албания']}]};
 const html=c.renderAdminSurvey(false);
 assert.doesNotMatch(html,/broadcast-step|<small>1<\/small>|<span>Кнопка [1-5]<\/span>/);
 assert.equal((html.match(/class="survey-answer-input"/g)||[]).length,5);
 assert.match(html,/aria-label="Вариант ответа 5"/);
});

test('reopening a pending bot capture reveals its question and blocks sending and editing',()=>{
 const {c,state}=harness();const survey={...c.defaultAdminSurvey(),enabled:true,questions:[{text:'Первый',options:['Да']},{text:'Второй',options:['Нет']}],capture:{id:'capture',target:'question',question:1}};
 c.setAdminBroadcastDraft({status:'draft',updatedAt:'waiting',survey},{forceSurvey:true});
 assert.equal(state.adminSurveyOpenQuestion,1);assert.equal(c.adminBroadcastReadiness().ready,false);
 c.editAdminSurvey('removeQuestion',1);assert.equal(state.adminSurveyDraft.questions.length,2);
 const html=c.renderAdminSurvey(true);assert.match(html,/survey-capture-cancel/);assert.match(html,/Ждём сообщение/);
});

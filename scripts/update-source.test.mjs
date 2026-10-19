import test from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs';
import os from 'node:os';
import path from 'node:path';
import {spawnSync} from 'node:child_process';
import {fileURLToPath} from 'node:url';

const scripts=path.dirname(fileURLToPath(import.meta.url));
const bash=process.platform==='win32'?'C:/Program Files/Git/bin/bash.exe':'bash';
const baseCompose=`services:
  bot:
    image: link-bot:local
    restart: unless-stopped
    container_name: link-bot
    environment:
      TZ: UTC
    # deployment mounts
    # keep assets and media
    volumes:
      - ./assets:/assets:ro
      - uploads:/uploads
volumes:
  uploads:
`;
const baseCaddy=`{$PUBLIC_HOST:localhost} {
  encode gzip
  # local proxy settings
  # default routes
  # health routing
  # cabinet routes
  # support routes
  # forwarding
  reverse_proxy bot:8080
}
`;

function command(cwd,exe,args){
 const result=spawnSync(exe,args,{cwd,encoding:'utf8',windowsHide:true});
 assert.equal(result.status,0,`${exe} ${args.join(' ')}\n${result.stdout}\n${result.stderr}`);
 return result.stdout.trim();
}
function fixture(t){
 const root=fs.mkdtempSync(path.join(os.tmpdir(),'link-bot-update-'));
 t.after(()=>fs.rmSync(root,{recursive:true,force:true}));
 const origin=path.join(root,'origin'),repo=path.join(root,'checkout');fs.mkdirSync(origin);
 const git=(cwd,...args)=>command(cwd,'git',args);
 git(origin,'init','--initial-branch=main');git(origin,'config','user.name','Update Test');git(origin,'config','user.email','update@example.test');git(origin,'config','core.autocrlf','false');
 const write=(cwd,file,value)=>fs.writeFileSync(path.join(cwd,file),value);
 write(origin,'docker-compose.yaml',baseCompose);write(origin,'Caddyfile',baseCaddy);write(origin,'app.go','old source\n');
 git(origin,'add','.');git(origin,'commit','-m','base');git(root,'-c','core.autocrlf=false','clone',origin,repo);git(repo,'config','core.autocrlf','false');
 const head=git(repo,'rev-parse','HEAD');
 const bin=path.join(repo,'test-bin');fs.mkdirSync(bin);
 write(bin,'docker',`#!/usr/bin/env bash
set -eu
printf '%s\\n' "$*" >> docker-checks.log
if [[ $1 == compose ]]; then
  if [[ -f edit-during-check ]]; then printf '# concurrent edit\\n' >> Caddyfile; fi
  [[ ! -f fail-compose ]]
elif [[ $1 == inspect ]]; then printf 'true\\n'
elif [[ $1 == exec ]]; then cat >/dev/null; [[ ! -f fail-caddy ]]
else exit 99
fi
`);fs.chmodSync(path.join(bin,'docker'),0o755);
 return {
  root,repo,origin,head,git,
  local(file,text){write(repo,file,text)},
  read(file){return fs.readFileSync(path.join(repo,file),'utf8')},
  remote(changes){for(const [file,value]of Object.entries(changes))write(origin,file,value);git(origin,'add','.');git(origin,'commit','-m','upstream update');return git(origin,'rev-parse','HEAD')},
  run(){return spawnSync(bash,['-c','export PATH="$PWD/test-bin:$PATH"; bash "$1"','update-test',path.join(scripts,'update-source.sh').replaceAll('\\','/')],{cwd:repo,encoding:'utf8',windowsHide:true})},
  backups(){const dir=path.join(repo,'.git','link-bot-update-backups');return fs.existsSync(dir)?fs.readdirSync(dir).map(name=>path.join(dir,name)):[]},
 };
}

test('clean checkout fast-forwards without stashing or requiring Docker',t=>{
 const f=fixture(t),target=f.remote({'app.go':'new source\n'}),result=f.run();
 assert.equal(result.status,0,result.stderr);assert.equal(f.git(f.repo,'rev-parse','HEAD'),target);assert.equal(f.read('app.go'),'new source\n');assert.equal(f.backups().length,0);assert.ok(!fs.existsSync(path.join(f.repo,'docker-checks.log')));
});

test('local Compose and Caddy settings survive updates and repeated runs',t=>{
 const f=fixture(t),compose=baseCompose.replace('container_name: link-bot','container_name: custom-bot'),caddy=baseCaddy.replace('{$PUBLIC_HOST:localhost}','my.example.test');
 f.local('docker-compose.yaml',compose);f.local('Caddyfile',caddy);f.local('.env','PRIVATE_TEST_VALUE=kept\n');f.local('custom-banner.png','user image');
 const target=f.remote({'docker-compose.yaml':baseCompose.replace('      - uploads:/uploads','      - uploads:/uploads\n      - ./.update-agent:/run/link-bot-updater:ro'),'Caddyfile':baseCaddy.replace('  reverse_proxy bot:8080','  header X-Content-Type-Options nosniff\n  reverse_proxy bot:8080')});
 const result=f.run();assert.equal(result.status,0,result.stdout+'\n'+result.stderr);
 assert.equal(f.git(f.repo,'rev-parse','HEAD'),target);assert.match(f.read('docker-compose.yaml'),/container_name: custom-bot/);assert.match(f.read('docker-compose.yaml'),/update-agent/);assert.match(f.read('Caddyfile'),/my.example.test/);assert.match(f.read('Caddyfile'),/nosniff/);
 assert.equal(f.read('.env'),'PRIVATE_TEST_VALUE=kept\n');assert.equal(f.read('custom-banner.png'),'user image');
 const backups=f.backups();assert.equal(backups.length,1);assert.equal(fs.readFileSync(path.join(backups[0],'docker-compose.yaml.local'),'utf8'),compose);assert.equal(fs.readFileSync(path.join(backups[0],'Caddyfile.local'),'utf8'),caddy);
 assert.match(f.read('docker-checks.log'),/config --quiet/);assert.match(f.read('docker-checks.log'),/caddy adapt/);
 assert.equal(f.git(f.repo,'stash','list'),'');assert.equal(f.run().status,0);assert.equal(f.git(f.repo,'rev-parse','HEAD'),target);
});

test('conflicting config leaves all live files and HEAD unchanged',t=>{
 const f=fixture(t),compose=baseCompose.replace('image: link-bot:local','image: private:local'),caddy=baseCaddy.replace('localhost','local.example.test');f.local('docker-compose.yaml',compose);f.local('Caddyfile',caddy);
 f.remote({'docker-compose.yaml':baseCompose.replace('image: link-bot:local','image: upstream:new')});
 const result=f.run();assert.equal(result.status,21,result.stderr);assert.equal(f.read('docker-compose.yaml'),compose);assert.equal(f.read('Caddyfile'),caddy);assert.equal(f.git(f.repo,'rev-parse','HEAD'),f.head);assert.equal(f.git(f.repo,'diff','--cached','--name-only'),'');assert.equal(f.backups().length,1);
});

for(const fail of ['compose','caddy'])test(`failed ${fail} validation does not modify checkout or restart containers`,t=>{
 const f=fixture(t),caddy=baseCaddy.replace('localhost','my.example.test');f.local('Caddyfile',caddy);f.local(`fail-${fail}`,'');f.remote({'app.go':'new source\n'});
 const result=f.run();assert.equal(result.status,22,result.stderr);assert.equal(f.git(f.repo,'rev-parse','HEAD'),f.head);assert.equal(f.read('Caddyfile'),caddy);assert.doesNotMatch(f.read('docker-checks.log'),/\b(up|restart|stop)\b/);
});

test('failed fast-forward restores local configs and preserves colliding untracked files',t=>{
 const f=fixture(t),compose=baseCompose.replace('UTC','Europe/Moscow');f.local('docker-compose.yaml',compose);f.local('collision.txt','local user file');f.remote({'collision.txt':'incoming file'});
 const result=f.run();assert.notEqual(result.status,0);assert.equal(f.read('docker-compose.yaml'),compose);assert.equal(f.git(f.repo,'rev-parse','HEAD'),f.head);assert.equal(f.read('collision.txt'),'local user file');assert.equal(f.git(f.repo,'diff','--cached','--name-only'),'');
});

test('concurrent user edits stop the update without overwriting the new edit',t=>{
 const f=fixture(t),caddy=baseCaddy.replace('localhost','my.example.test');f.local('Caddyfile',caddy);f.local('edit-during-check','');f.remote({'app.go':'new source\n'});
 const result=f.run();assert.equal(result.status,23,result.stderr);assert.equal(f.read('Caddyfile'),caddy+'# concurrent edit\n');assert.equal(f.git(f.repo,'rev-parse','HEAD'),f.head);
});

for(const staged of [false,true])test(`source edits and ${staged?'staged':'unstaged'} changes stay untouched`,t=>{
 const f=fixture(t);f.local(staged?'docker-compose.yaml':'app.go',staged?baseCompose.replace('UTC','Europe/Moscow'):'local code\n');if(staged)f.git(f.repo,'add','docker-compose.yaml');
 const result=f.run();assert.equal(result.status,20,result.stderr);assert.equal(f.git(f.repo,'rev-parse','HEAD'),f.head);if(staged)assert.equal(f.git(f.repo,'diff','--cached','--name-only'),'docker-compose.yaml');else assert.equal(f.read('app.go'),'local code\n');
});

test('agent refresh starts inactive services, replaces idle old agents, and keeps its own job alive',t=>{
 const root=fs.mkdtempSync(path.join(os.tmpdir(),'link-bot-refresh-'));t.after(()=>fs.rmSync(root,{recursive:true,force:true}));
 const service='link-bot-updater-012345abcdef.service',bin=path.join(root,'test-bin');fs.mkdirSync(bin);
 fs.writeFileSync(path.join(bin,'systemctl'),`#!/usr/bin/env bash
case "$1" in
is-active) [[ -f active ]] ;;
show) if [[ -f own-job ]]; then printf '%s\\n' "$PPID"; else printf '99999999\\n'; fi ;;
start|restart) printf '%s\\n' "$1" >> actions ;;
*) exit 99 ;;
esac
`);fs.chmodSync(path.join(bin,'systemctl'),0o755);
 // Linux ps supports -o; the Windows Git Bash ps implementation does not.
 if(process.platform==='win32'){fs.writeFileSync(path.join(bin,'ps'),'#!/usr/bin/env bash\nprintf "1\\n"\n');fs.chmodSync(path.join(bin,'ps'),0o755)}
 const run=()=>spawnSync(bash,['-c','export PATH="$PWD/test-bin:$PATH"; bash "$1" "$2"','refresh-test',path.join(scripts,'refresh-update-agent.sh').replaceAll('\\','/'),service],{cwd:root,encoding:'utf8',windowsHide:true});
 assert.equal(run().status,0);assert.equal(fs.readFileSync(path.join(root,'actions'),'utf8'),'start\n');
 fs.writeFileSync(path.join(root,'active'),'');assert.equal(run().status,0);assert.equal(fs.readFileSync(path.join(root,'actions'),'utf8'),'start\nrestart\n');
 fs.writeFileSync(path.join(root,'own-job'),'');const result=run();assert.equal(result.status,0,result.stderr);assert.equal(fs.readFileSync(path.join(root,'actions'),'utf8'),'start\nrestart\n');
});

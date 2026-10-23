import test from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs';
import os from 'node:os';
import path from 'node:path';
import {spawnSync} from 'node:child_process';

const script=path.resolve('scripts/package_wechat_cloudrun.py');
test('cloud source archive excludes credentials, data, client and large media',t=>{
 const tmp=fs.mkdtempSync(path.join(os.tmpdir(),'wechat-cloudrun-'));
 t.after(()=>fs.rmSync(tmp,{recursive:true,force:true}));
 const files=['go.mod','go.sum','cmd/wechat-server/main.go','internal/game/store.go','content/book1.json','Dockerfile','deploy/wechat/cloudrun-entrypoint.sh','deploy/wechat/release-id.txt','.env','data/save.json','miniprogram/app.js','web/assets/scenes/lesson.png','private.key'];
 for(const f of files){fs.mkdirSync(path.dirname(path.join(tmp,f)),{recursive:true});fs.writeFileSync(path.join(tmp,f),f);}
 const output=path.join(tmp,'bundle.zip');
 const p=spawnSync('python3',[script,'--root',tmp,'--output',output],{encoding:'utf8'});
 assert.equal(p.status,0,p.stderr);
 const names=JSON.parse(spawnSync('python3',['-c','import zipfile,json,sys; print(json.dumps(zipfile.ZipFile(sys.argv[1]).namelist()))',output],{encoding:'utf8'}).stdout);
 assert.deepEqual(names.sort(),['Dockerfile','deploy/wechat/cloudrun-entrypoint.sh','deploy/wechat/release-id.txt','go.mod','go.sum','cmd/wechat-server/main.go','internal/game/store.go','content/book1.json'].sort());
});
test('cloud archive refuses symlinked source outside project',t=>{
 const tmp=fs.mkdtempSync(path.join(os.tmpdir(),'wechat-cloudrun-'));
 t.after(()=>fs.rmSync(tmp,{recursive:true,force:true}));
 fs.symlinkSync('/etc/hosts',path.join(tmp,'go.mod'));
 const p=spawnSync('python3',[script,'--root',tmp,'--output',path.join(tmp,'bundle.zip')],{encoding:'utf8'});
 assert.notEqual(p.status,0);assert.match(p.stderr,/symlink|符号链接/);
 assert.equal(fs.existsSync(path.join(tmp,'bundle.zip')),false);
});

import test from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs';
import os from 'node:os';
import path from 'node:path';
import {createRequire} from 'node:module';
const require=createRequire(import.meta.url);
const {prepareUpload,stageProject,performUpload}=require('../scripts/upload_wechat.cjs');

function fixture(t) {
 const dir=fs.mkdtempSync(path.join(os.tmpdir(),'wechat-upload-'));
 t.after(()=>fs.rmSync(dir,{recursive:true,force:true}));
 const root=path.join(dir,'repo');fs.mkdirSync(path.join(root,'miniprogram/pages/home'),{recursive:true});
 const write=(file,value)=>fs.writeFileSync(path.join(root,file),typeof value==='string'?value:JSON.stringify(value));
 write('project.config.json',{appid:'wx1f23b102bcb34ed7',miniprogramRoot:'miniprogram/',setting:{urlCheck:true}});
 write('miniprogram/config.json',{apiBase:'https://learning.example.net',devLogin:false});
 write('miniprogram/app.json',{pages:['pages/home/home']});write('miniprogram/app.js','App({})');write('miniprogram/app.wxss','');
 for(const ext of ['js','json','wxml','wxss'])write(`miniprogram/pages/home/home.${ext}`,ext==='json'?'{}':'');
 write('server.env','APP_SECRET=should-never-be-uploaded');
 const key=path.join(dir,'private.key');fs.writeFileSync(key,'TEST ONLY');
 return {root,key,env:{WECHAT_APP_ID:'wx1f23b102bcb34ed7',WECHAT_UPLOAD_KEY_PATH:key,WECHAT_VERSION:'0.1.0'}};
}
test('upload refuses missing credentials and mismatched app ID before calling SDK',async t=>{
 const {root,env}=fixture(t);
 await assert.rejects(prepareUpload(root,{}),/WECHAT_APP_ID/);
 await assert.rejects(prepareUpload(root,{...env,WECHAT_APP_ID:'wx0000000000000000'}),/AppID/);
 await assert.rejects(prepareUpload(root,{...env,WECHAT_UPLOAD_KEY_PATH:''}),/WECHAT_UPLOAD_KEY_PATH/);
});
test('upload refuses private keys stored inside source repository',async t=>{
 const {root,env}=fixture(t),key=path.join(root,'private.key');fs.writeFileSync(key,'TEST ONLY');
 await assert.rejects(prepareUpload(root,{...env,WECHAT_UPLOAD_KEY_PATH:key}),/仓库外/);
});
test('upload cannot bypass release gate in development mode',async t=>{
 const {root,env}=fixture(t);fs.writeFileSync(path.join(root,'miniprogram/config.json'),JSON.stringify({apiBase:'http://127.0.0.1:8093',devLogin:true}));
 await assert.rejects(prepareUpload(root,env),/devLogin/);
});
test('staged upload contains only mini-program and project config, never server sources or keys',async t=>{
 const {root,env}=fixture(t),options=await prepareUpload(root,env),stage=stageProject(options);
 t.after(()=>fs.rmSync(stage,{recursive:true,force:true}));
 assert.deepEqual(fs.readdirSync(stage).sort(),['miniprogram','project.config.json']);
 assert.equal(fs.existsSync(path.join(stage,'miniprogram/app.js')),true);
 assert.equal(fs.existsSync(path.join(stage,'server.env')),false);
 assert.equal(options.appid,env.WECHAT_APP_ID);
});
test('upload refuses environment files mistakenly copied into native source',async t=>{
 const {root,env}=fixture(t);fs.writeFileSync(path.join(root,'miniprogram/server.env'),'WECHAT_APP_SECRET=not-a-real-secret');
 await assert.rejects(prepareUpload(root,env),/server.env/);
});
test('successful platform upload produces a receipt while removing staged files',async t=>{
 const {root,env}=fixture(t),options=await prepareUpload(root,env);let staged;
 const ci={Project:class{constructor(p){staged=p.projectPath;}},upload:async()=>({strUint64Version:'12345'})};
 const result=await performUpload(options,ci);
 assert.equal(result.status,'development-uploaded');assert.equal(result.platformVersion,'12345');assert.equal(result.experiencePublished,false);
 assert.equal(fs.existsSync(staged),false);
 const receipt=fs.readFileSync(result.receiptPath,'utf8');assert.doesNotMatch(receipt,/PRIVATE KEY|TEST ONLY|privateKeyPath/);
});
test('rejected upload cleans up and reports allowed diagnostic fields without creating a receipt',async t=>{
 const {root,env}=fixture(t),options=await prepareUpload(root,env);let staged;
 const ci={Project:class{constructor(p){staged=p.projectPath;}},upload:async()=>{throw new Error('invalid ip: 192.0.2.5; private-token-must-not-leak');}};
 await assert.rejects(performUpload(options,ci),error=>{assert.match(error.message,/192\.0\.2\.5/);assert.doesNotMatch(error.message,/private-token/);return true;});
 assert.equal(fs.existsSync(staged),false);assert.equal(fs.existsSync(path.join(root,'tmp')),false);
});

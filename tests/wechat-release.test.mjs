import test from 'node:test';
import assert from 'node:assert/strict';
import {mkdtempSync,mkdirSync,writeFileSync,rmSync,renameSync,symlinkSync} from 'node:fs';
import {tmpdir} from 'node:os';
import {join} from 'node:path';
import {checkProject} from '../scripts/check_wechat.mjs';

function fixture(t, config={apiBase:'https://learning.example.net',devLogin:false}) {
 const root=mkdtempSync(join(tmpdir(),'wechat-release-'));
 t.after(()=>rmSync(root,{recursive:true,force:true}));
 mkdirSync(join(root,'miniprogram/pages/index'),{recursive:true});
 writeFileSync(join(root,'project.config.json'),JSON.stringify({appid:'wx1f23b102bcb34ed7',miniprogramRoot:'miniprogram/',setting:{urlCheck:true}}));
 writeFileSync(join(root,'miniprogram/config.json'),JSON.stringify(config));
 writeFileSync(join(root,'miniprogram/config.js'),`module.exports = ${JSON.stringify(config,null,2)};\n`);
 writeFileSync(join(root,'miniprogram/app.json'),JSON.stringify({pages:['pages/index/index']}));
 for(const ext of ['js','json','wxml','wxss'])writeFileSync(join(root,`miniprogram/pages/index/index.${ext}`),ext==='json'?'{}':'');
 writeFileSync(join(root,'miniprogram/app.js'),'App({})');
 writeFileSync(join(root,'miniprogram/app.wxss'),'page{}');
 return root;
}
test('release preflight rejects local development and IP endpoints',t=>{
 const r=checkProject(fixture(t,{apiBase:'http://127.0.0.1:8093',devLogin:true}),{release:true});
 assert.equal(r.ok,false); assert.ok(r.errors.some(x=>x.includes('HTTPS'))); assert.ok(r.errors.some(x=>x.includes('devLogin')));
 const ip=checkProject(fixture(t,{apiBase:'https://47.111.138.33',devLogin:false}),{release:true});
 assert.equal(ip.ok,false);
});
test('structural checks work locally but cannot be mistaken for release readiness',t=>{
 const root=fixture(t,{apiBase:'http://127.0.0.1:8093',devLogin:true});
 assert.equal(checkProject(root,{release:false}).ok,true);
 assert.equal(checkProject(root,{release:true}).ok,false);
});
test('reject missing page files and bundled credentials',t=>{
 const root=fixture(t);rmSync(join(root,'miniprogram/pages/index/index.wxml'));
 writeFileSync(join(root,'miniprogram/private.key'),'-----BEGIN PRIVATE KEY-----\nsecret');
 const r=checkProject(root,{release:true});assert.equal(r.ok,false);
 assert.ok(r.errors.some(x=>x.includes('index.wxml')));assert.ok(r.errors.some(x=>x.includes('private.key')));
});
test('detect broken image reference and oversized package',t=>{
 const root=fixture(t);writeFileSync(join(root,'miniprogram/pages/index/index.wxml'),'<image src="/missing.png"/>');
 writeFileSync(join(root,'miniprogram/large.png'),Buffer.alloc(1024*1024+1));
 const r=checkProject(root,{release:false});assert.equal(r.ok,false);
 assert.ok(r.errors.some(x=>x.includes('missing.png')));assert.ok(r.errors.some(x=>x.includes('1 MiB')));
});
test('valid project returns files and byte size without claiming platform publication',t=>{
 const r=checkProject(fixture(t),{release:true});assert.equal(r.ok,true,JSON.stringify(r.errors));
 assert.ok(r.bytes>0);assert.ok(r.files.includes('pages/index/index.wxml'));assert.equal(r.published,undefined);
});
test('rejects runtime configuration that differs from checked JSON',t=>{
 const root=fixture(t);
 writeFileSync(join(root,'miniprogram/config.js'),"module.exports = {apiBase:'https://other.example.net',devLogin:false};\n");
 const r=checkProject(root,{release:true});
 assert.equal(r.ok,false);assert.ok(r.errors.some(x=>x.includes('config.js')));
});
test('rejects environment files and test code copied into mini-program package',t=>{
 const root=fixture(t);mkdirSync(join(root,'miniprogram/tests'));
 writeFileSync(join(root,'miniprogram/tests/helper.test.js'),'const testOnly=true;');
 writeFileSync(join(root,'miniprogram/server.env'),'WECHAT_APP_SECRET=not-a-real-secret');
 const r=checkProject(root,{release:true});assert.equal(r.ok,false);
 assert.ok(r.errors.some(x=>x.includes('server.env')));assert.ok(r.errors.some(x=>x.includes('helper.test.js')));
});
test('rejects mini-program root symlink before reading external project files',t=>{
 const root=fixture(t),outside=mkdtempSync(join(tmpdir(),'wechat-external-'));
 t.after(()=>rmSync(outside,{recursive:true,force:true}));
 renameSync(join(root,'miniprogram'),join(outside,'mini'));symlinkSync(join(outside,'mini'),join(root,'miniprogram'));
 const r=checkProject(root);assert.equal(r.ok,false);assert.ok(r.errors.some(x=>x.includes('符号链接')));
 assert.equal(r.files.length,0);
});
test('public HTTP release rejects CloudRun default test domain, private cloud route requires named environment and service',t=>{
 const defaults={apiBase:'https://golang-8467-333734-12-1514891213.sh.run.tcloudbase.com',devLogin:false};
 assert.equal(checkProject(fixture(t,defaults),{release:true}).ok,false);
 const cloud={...defaults,transport:'cloudrun',cloudEnv:'prod-d2g6ceg6hee15e89b',cloudService:'golang-8467'};
 assert.equal(checkProject(fixture(t,cloud),{release:true}).ok,true);
 for(const invalid of [{...cloud,cloudEnv:''},{...cloud,cloudService:''},{...cloud,cloudEnv:'https://example.org'},{...cloud,transport:'typo'}])assert.equal(checkProject(fixture(t,invalid),{release:true}).ok,false);
});

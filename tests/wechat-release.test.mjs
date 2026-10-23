import test from 'node:test';
import assert from 'node:assert/strict';
import {mkdtempSync,mkdirSync,writeFileSync,rmSync} from 'node:fs';
import {tmpdir} from 'node:os';
import {join} from 'node:path';
import {checkProject} from '../scripts/check_wechat.mjs';

function fixture(t, config={apiBase:'https://learning.example.net',devLogin:false}) {
 const root=mkdtempSync(join(tmpdir(),'wechat-release-'));
 t.after(()=>rmSync(root,{recursive:true,force:true}));
 mkdirSync(join(root,'miniprogram/pages/index'),{recursive:true});
 writeFileSync(join(root,'project.config.json'),JSON.stringify({appid:'wx1f23b102bcb34ed7',miniprogramRoot:'miniprogram/',setting:{urlCheck:true}}));
 writeFileSync(join(root,'miniprogram/config.json'),JSON.stringify(config));
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

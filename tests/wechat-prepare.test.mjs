import test from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs';
import os from 'node:os';
import path from 'node:path';
import {createRequire} from 'node:module';
import {prepareProject} from '../scripts/prepare_wechat.mjs';
const require=createRequire(import.meta.url);
const {syncRuntimeConfig}=require('../scripts/sync_wechat_config.cjs');
test('prepare creates an importable project without upload key and supports cloud profile',t=>{
 const dir=fs.mkdtempSync(path.join(os.tmpdir(),'wechat-prepare-'));t.after(()=>fs.rmSync(dir,{recursive:true,force:true}));
 const root=path.resolve('.'),output=path.join(dir,'ready');
 const result=prepareProject(root,output,{configPath:path.join(root,'deploy/wechat/miniprogram.config.cloudrun.json'),release:true});
 assert.equal(result.ok,true);assert.equal(result.published,false);
 assert.deepEqual(fs.readdirSync(output).sort(),['miniprogram','project.config.json']);
 const config=JSON.parse(fs.readFileSync(path.join(output,'miniprogram/config.json'),'utf8'));
 assert.equal(config.devLogin,false);assert.equal(config.transport,'cloudrun');assert.equal(config.cloudService,'golang-8467');assert.match(config.apiBase,/run\.tcloudbase\.com$/);
 const runtimeConfig=require(path.join(output,'miniprogram/config.js'));
 assert.deepEqual(runtimeConfig,config);
 for(const file of ['app.js','pages/settings/settings.js']) {
  assert.doesNotMatch(fs.readFileSync(path.join(output,'miniprogram',file),'utf8'),/require\([^)]*config\.json/);
 }
 assert.ok(fs.existsSync(path.join(output,'miniprogram/pages/lesson/lesson.wxml')));
 assert.throws(()=>prepareProject(root,output),/已存在/);
});
test('syncs the local runtime module after editing JSON configuration',t=>{
 const dir=fs.mkdtempSync(path.join(os.tmpdir(),'wechat-config-'));t.after(()=>fs.rmSync(dir,{recursive:true,force:true}));
 fs.mkdirSync(path.join(dir,'miniprogram'));
 const config={apiBase:'https://example.org',devLogin:false};
 fs.writeFileSync(path.join(dir,'miniprogram/config.json'),JSON.stringify(config));
 fs.writeFileSync(path.join(dir,'miniprogram/config.js'),'module.exports = {};\n');
 syncRuntimeConfig(dir);
 assert.deepEqual(require(path.join(dir,'miniprogram/config.js')),config);
});

import test from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs';
import os from 'node:os';
import path from 'node:path';
import {prepareProject} from '../scripts/prepare_wechat.mjs';
test('prepare creates an importable project without upload key and supports cloud profile',t=>{
 const dir=fs.mkdtempSync(path.join(os.tmpdir(),'wechat-prepare-'));t.after(()=>fs.rmSync(dir,{recursive:true,force:true}));
 const root=path.resolve('.'),output=path.join(dir,'ready');
 const result=prepareProject(root,output,{configPath:path.join(root,'deploy/wechat/miniprogram.config.cloudrun.json'),release:true});
 assert.equal(result.ok,true);assert.equal(result.published,false);
 assert.deepEqual(fs.readdirSync(output).sort(),['miniprogram','project.config.json']);
 const config=JSON.parse(fs.readFileSync(path.join(output,'miniprogram/config.json'),'utf8'));
 assert.equal(config.devLogin,false);assert.match(config.apiBase,/run\.tcloudbase\.com$/);
 assert.ok(fs.existsSync(path.join(output,'miniprogram/pages/lesson/lesson.wxml')));
 assert.throws(()=>prepareProject(root,output),/已存在/);
});

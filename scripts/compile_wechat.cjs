#!/usr/bin/env node
'use strict';
// Local WXML/WXSS validation using the compiler shipped in pinned miniprogram-ci.
// This is not a developer-tool simulator, wx.login test, or a platform upload.
const fs=require('node:fs');
const path=require('node:path');
const vm=require('node:vm');

async function main() {
 const root=path.resolve(__dirname,'..');
 const {checkProject}=await import('./check_wechat.mjs');
 const check=checkProject(root);
 if(!check.ok)throw new Error(check.errors.join('\n'));
 let compiler;
 try {compiler=require('../tools/wechat-ci/node_modules/miniprogram-ci/dist/modules/fullcompiler/trans/wcc/lib');}
 catch {throw new Error('请先用 Node >=20.19 执行 npm ci --prefix tools/wechat-ci');}
 const mini=path.resolve(root,JSON.parse(fs.readFileSync(path.join(root,'project.config.json'),'utf8')).miniprogramRoot);
 const app=JSON.parse(fs.readFileSync(path.join(mini,'app.json'),'utf8'));
 for(const file of check.files) {
  const text=fs.readFileSync(path.join(mini,file),'utf8');
  if(file.endsWith('.js'))new vm.Script(text,{filename:file});
  if(file.endsWith('.json'))JSON.parse(text);
 }
 const wxml=check.files.filter(f=>f.endsWith('.wxml')).map(f=>'./'+f);
 const wxss=[...app.pages.map(f=>'./'+f+'.wxss'),'./app.wxss'];
 await compiler.wcc({cwd:mini,files:wxml});
 await compiler.wcsc({cwd:mini,files:wxss,pageCount:app.pages.length});
 console.log(JSON.stringify({pages:app.pages.length,wxml:wxml.length,wxss:wxss.length,sourceBytes:check.bytes,compiled:true,deviceTested:false,published:false}));
}
main().catch(error=>{console.error(error.message);process.exitCode=1;});

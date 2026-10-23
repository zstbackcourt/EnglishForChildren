#!/usr/bin/env node
import fs from 'node:fs';
import path from 'node:path';
import {fileURLToPath} from 'node:url';
import {createRequire} from 'node:module';
import {checkProject} from './check_wechat.mjs';
const {stageProject}=createRequire(import.meta.url)('./upload_wechat.cjs');

export function prepareProject(root,output,{configPath,release=false}={}) {
 root=fs.realpathSync(root);output=path.resolve(output);
 if(fs.existsSync(output))throw new Error('输出目录已存在，请选择新目录以保留现有文件');
 const check=checkProject(root);
 if(!check.ok)throw new Error(check.errors.join('\n'));
 const project=JSON.parse(fs.readFileSync(path.join(root,'project.config.json'),'utf8'));
 const config=configPath?JSON.parse(fs.readFileSync(configPath,'utf8')):null;
 fs.mkdirSync(path.dirname(output),{recursive:true});
 stageProject({root,project,files:check.files},output);
 try {
  if(config)fs.writeFileSync(path.join(output,'miniprogram/config.json'),JSON.stringify(config,null,2)+'\n');
  const result=checkProject(output,{release});
  if(!result.ok)throw new Error(result.errors.join('\n'));
  return {...result,directory:output,published:false};
 } catch(error) {fs.rmSync(output,{recursive:true,force:true});throw error;}
}
if(process.argv[1]&&path.resolve(process.argv[1])===fileURLToPath(import.meta.url)) {
 try {
  const root=path.resolve(path.dirname(fileURLToPath(import.meta.url)),'..');
  const index=process.argv.indexOf('--output'),output=index>=0?process.argv[index+1]:path.join(root,'tmp',`wechat-import-${Date.now()}`);
  const profile=process.argv.includes('--cloudrun')?path.join(root,'deploy/wechat/miniprogram.config.cloudrun.json'):undefined;
  const result=prepareProject(root,output,{configPath:profile,release:process.argv.includes('--release')});
  console.log(JSON.stringify(result,null,2));
 } catch(error) {console.error(error.message);process.exitCode=1;}
}

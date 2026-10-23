#!/usr/bin/env node
'use strict';
const fs=require('node:fs');
const os=require('node:os');
const path=require('node:path');

async function prepareUpload(root,env=process.env) {
 const {checkProject}=await import('./check_wechat.mjs');
 root=fs.realpathSync(root);
 if(!env.WECHAT_APP_ID)throw new Error('请设置 WECHAT_APP_ID');
 const result=checkProject(root,{release:true});
 if(!result.ok)throw new Error(result.errors.join('\n'));
 if(env.WECHAT_APP_ID!==result.appid)throw new Error('环境中的 AppID 与 project.config.json 不一致');
 if(!env.WECHAT_UPLOAD_KEY_PATH)throw new Error('请设置仓库外的 WECHAT_UPLOAD_KEY_PATH');
 let key;
 try{key=fs.realpathSync(env.WECHAT_UPLOAD_KEY_PATH);if(!fs.statSync(key).isFile())throw new Error();}
 catch{throw new Error('WECHAT_UPLOAD_KEY_PATH 必须指向可读取的私钥文件');}
 if(key===root||key.startsWith(root+path.sep))throw new Error('上传密钥必须保存在仓库外');
 if(!/^\d+\.\d+\.\d+(?:[-.][A-Za-z0-9.-]+)?$/.test(env.WECHAT_VERSION||''))throw new Error('请设置 WECHAT_VERSION，例如 0.1.0');
 const project=JSON.parse(fs.readFileSync(path.join(root,'project.config.json'),'utf8'));
 return {root,appid:result.appid,privateKeyPath:key,version:env.WECHAT_VERSION,desc:env.WECHAT_DESCRIPTION||'英语冒险岛体验版',project,files:result.files,bytes:result.bytes};
}

function stageProject(options) {
 const dir=fs.mkdtempSync(path.join(os.tmpdir(),'english-wechat-upload-'));
 try {
  const source=path.resolve(options.root,options.project.miniprogramRoot);
  fs.mkdirSync(path.join(dir,'miniprogram'));
  // Copy only files inspected by preflight. No Go source, course archive or credentials.
  for(const rel of options.files) {
   const from=path.join(source,rel),to=path.join(dir,'miniprogram',rel);
   if(!fs.lstatSync(from).isFile())throw new Error('待上传文件发生变化，请重新检查');
   fs.mkdirSync(path.dirname(to),{recursive:true});fs.copyFileSync(from,to);
  }
  fs.writeFileSync(path.join(dir,'project.config.json'),JSON.stringify({...options.project,miniprogramRoot:'miniprogram/'},null,2));
  return dir;
 } catch(error) {fs.rmSync(dir,{recursive:true,force:true});throw error;}
}

async function main() {
 if(!process.argv.includes('--upload'))throw new Error('此命令会上传微信开发版本。准备完成后使用 node scripts/upload_wechat.cjs --upload；只检查配置请运行 node scripts/check_wechat.mjs --release。');
 const root=path.resolve(__dirname,'..');
 const options=await prepareUpload(root);
 let ci;
 try {ci=require('../tools/wechat-ci/node_modules/miniprogram-ci');}
 catch {throw new Error('请先运行 npm ci --prefix tools/wechat-ci 安装已锁定的官方上传工具');}
 const staged=stageProject(options);
 try {
  const project=new ci.Project({appid:options.appid,type:'miniProgram',projectPath:staged,privateKeyPath:options.privateKeyPath,ignores:['node_modules/**/*']});
  console.log(`正在上传 ${options.appid} / ${options.version}，源文件 ${options.bytes} 字节……`);
  try {
   await ci.upload({project,version:options.version,desc:options.desc,robot:1,setting:{useProjectConfig:true,es7:true,minify:true}});
  } catch {throw new Error('微信编译或上传失败。请在本机检查上传密钥、上传 IP 白名单及微信后台状态；未生成成功凭据。');}
  const receipt={appid:options.appid,version:options.version,uploadedAt:new Date().toISOString(),sourceBytes:options.bytes,status:'development-uploaded',experiencePublished:false};
  fs.mkdirSync(path.join(root,'tmp'),{recursive:true});
  fs.writeFileSync(path.join(root,'tmp/wechat-upload-receipt.json'),JSON.stringify(receipt,null,2));
  console.log('开发版本上传成功。请在微信后台版本管理中设为体验版，添加体验成员并用真机验收；上传成功不等于体验版已发布。');
 } finally {fs.rmSync(staged,{recursive:true,force:true});}
}
module.exports={prepareUpload,stageProject};
if(require.main===module)main().catch(error=>{console.error(error.message);process.exitCode=1;});

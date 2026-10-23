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

function runtimeConfigSource(config) { return `module.exports = ${JSON.stringify(config,null,2)};\n`; }

function stageProject(options,destination) {
 const dir=destination?path.resolve(destination):fs.mkdtempSync(path.join(os.tmpdir(),'english-wechat-upload-'));
 if(destination)fs.mkdirSync(dir);
 try {
  const source=path.resolve(options.root,options.project.miniprogramRoot);
  if(source===options.root||!source.startsWith(options.root+path.sep))throw new Error('代码目录必须位于项目内');
  fs.mkdirSync(path.join(dir,'miniprogram'));
  // Copy only files inspected by preflight. No Go source, course archive or credentials.
  for(const rel of options.files) {
   const from=path.join(source,rel),to=path.join(dir,'miniprogram',rel);
   if(!from.startsWith(source+path.sep)||!to.startsWith(path.join(dir,'miniprogram')+path.sep))throw new Error('代码文件路径无效');
   for(let p=from;p!==options.root;p=path.dirname(p))if(fs.lstatSync(p).isSymbolicLink())throw new Error('代码路径不能包含符号链接');
   if(!fs.lstatSync(from).isFile())throw new Error('待上传文件发生变化，请重新检查');
   fs.mkdirSync(path.dirname(to),{recursive:true});fs.copyFileSync(from,to);
  }
  const config=options.config||JSON.parse(fs.readFileSync(path.join(dir,'miniprogram/config.json'),'utf8'));
  if(options.config)fs.writeFileSync(path.join(dir,'miniprogram/config.json'),JSON.stringify(config,null,2)+'\n');
  fs.writeFileSync(path.join(dir,'miniprogram/config.js'),runtimeConfigSource(config));
  fs.writeFileSync(path.join(dir,'project.config.json'),JSON.stringify({...options.project,miniprogramRoot:'miniprogram/'},null,2));
  return dir;
 } catch(error) {fs.rmSync(dir,{recursive:true,force:true});throw error;}
}

async function performUpload(options,ci) {
 const staged=stageProject(options);
 try {
  const project=new ci.Project({appid:options.appid,type:'miniProgram',projectPath:staged,privateKeyPath:options.privateKeyPath,ignores:['node_modules/**/*']});
  let result;
  try {
   result=await ci.upload({project,version:options.version,desc:options.desc,robot:1,setting:{useProjectConfig:true,es7:true,minify:true},onProgressUpdate:()=>{}});
  } catch(error) {
   const message=String(error?.message||error?.errMsg||'').slice(0,16384);
   if(/invalid ip|ip.*white|ip.*allow|白名单/i.test(message)){
    const ips=[...new Set(message.match(/\b(?:\d{1,3}\.){3}\d{1,3}\b/g)||[])].filter(ip=>ip.split('.').every(n=>Number(n)<=255));
    throw new Error(`微信上传 IP 白名单拒绝${ips.length?'：'+ips.join(', '):''}。请为固定出口配置白名单；未生成成功凭据。`);
   }
   throw new Error('微信编译或上传失败。请检查上传密钥、上传 IP 白名单及微信后台状态；未生成成功凭据。');
  }
  const receipt={appid:options.appid,version:options.version,uploadedAt:new Date().toISOString(),sourceBytes:options.bytes,status:'development-uploaded',platformVersion:result?.strUint64Version||null,experiencePublished:false};
  fs.mkdirSync(path.join(options.root,'tmp'),{recursive:true});
  const receiptPath=path.join(options.root,`tmp/wechat-upload-${options.version}-${Date.now()}.json`);
  fs.writeFileSync(receiptPath,JSON.stringify(receipt,null,2),{mode:0o600});
  return {...receipt,receiptPath};
 } finally {fs.rmSync(staged,{recursive:true,force:true});}
}
async function main() {
 if(!process.argv.includes('--upload'))throw new Error('此命令会上传微信开发版本。使用 --upload 显式执行；只检查请运行 check_wechat.mjs --release。');
 const index=process.argv.indexOf('--project');
 if(index>=0&&!process.argv[index+1])throw new Error('--project 需要工程目录');
 const root=path.resolve(index>=0?process.argv[index+1]:path.join(__dirname,'..'));
 const options=await prepareUpload(root);
 let ci;
 try {ci=require('../tools/wechat-ci/node_modules/miniprogram-ci');}
 catch {throw new Error('请先运行 npm ci --prefix tools/wechat-ci 安装已锁定的官方上传工具');}
 console.log(`正在上传 ${options.appid} / ${options.version}，源文件 ${options.bytes} 字节……`);
 const original={};
 for(const name of ['log','warn','info','debug','error']){original[name]=console[name];console[name]=()=>{};}
 let receipt;
 try {receipt=await performUpload(options,ci);}
 finally {Object.assign(console,original);}
 console.log(JSON.stringify(receipt));
 console.log('开发版本上传成功；仍需在微信后台设为体验版、添加体验成员并用真机验收。');
}
module.exports={prepareUpload,stageProject,performUpload,runtimeConfigSource};
// The SDK can retain compiler workers after a failed upload. This CLI owns them.
if(require.main===module)main().then(()=>process.exit(0)).catch(error=>{console.error(error.message);process.exit(1);});

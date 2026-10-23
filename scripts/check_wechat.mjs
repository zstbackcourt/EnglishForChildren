import fs from 'node:fs';
import path from 'node:path';
import {fileURLToPath} from 'node:url';
import {isIP} from 'node:net';

export function checkProject(projectRoot,{release=false}={}) {
 const root=path.resolve(projectRoot),errors=[],files=[];
 let bytes=0;
 const readJSON=p=>{try{return JSON.parse(fs.readFileSync(p,'utf8'))}catch{errors.push(`无法读取 JSON：${path.relative(root,p)}`);return {}}};
 const project=readJSON(path.join(root,'project.config.json'));
 const mini=path.resolve(root,project.miniprogramRoot||'miniprogram');
 if(mini===root||!mini.startsWith(root+path.sep))return {ok:false,errors:['miniprogramRoot 必须是项目内独立子目录'],files,bytes};
 const config=readJSON(path.join(mini,'config.json'));
 const app=readJSON(path.join(mini,'app.json'));
 function visit(dir) {
  if(!fs.existsSync(dir)){errors.push(`目录不存在：${path.relative(root,dir)}`);return}
  for(const e of fs.readdirSync(dir,{withFileTypes:true})) {
   const p=path.join(dir,e.name),rel=path.relative(mini,p);
   if(e.isSymbolicLink()){errors.push(`代码包不能含符号链接：${rel}`);continue}
   if(e.isDirectory()){visit(p);continue}
   files.push(rel);const b=fs.readFileSync(p);bytes+=b.length;
   if(/(^|[/\\])(?:\.env(?:\..*)?|[^/\\]+\.(?:pem|key)|save\.json)$/.test(rel)||b.includes(Buffer.from('-----BEGIN PRIVATE KEY-----'))||b.includes(Buffer.from('-----BEGIN RSA PRIVATE KEY-----')))errors.push(`代码包包含凭证或存档：${rel}`);
   if(/\.(mp3|lrc|pdf|zip|tar|gz)$/i.test(rel))errors.push(`大素材必须远程加载：${rel}`);
   if(e.name.endsWith('.wxml'))for(const match of b.toString().matchAll(/\bsrc\s*=\s*["']([^"']+)["']/g)) {
    const src=match[1];if(src.includes('{{')||/^(https?:|data:)/.test(src))continue;
    const target=src.startsWith('/')?path.join(mini,src):path.resolve(dir,src);
    if(!target.startsWith(mini+path.sep)||!fs.existsSync(target))errors.push(`本地图片不存在：${rel} -> ${src}`);
   }
  }
 }
 visit(mini);
 for(const f of ['app.js','app.wxss'])if(!fs.existsSync(path.join(mini,f)))errors.push(`缺少 ${f}`);
 if(!Array.isArray(app.pages)||!app.pages.length)errors.push('app.json 至少配置一个页面');
 for(const page of Array.isArray(app.pages)?app.pages:[]) {
  if(typeof page!=='string'||page.includes('..')||page.startsWith('/')){errors.push('页面路径无效');continue}
  for(const ext of ['js','json','wxml','wxss'])if(!fs.existsSync(path.join(mini,`${page}.${ext}`)))errors.push(`缺少页面文件：${page}.${ext}`);
 }
 if(bytes>1024*1024)errors.push('代码包超过项目设定的 1 MiB 预算；图片和音频应远程加载');
 if(!/^wx[0-9a-f]{16}$/.test(project.appid||''))errors.push('请配置真实小程序 AppID');
 let base;try{base=new URL(config.apiBase)}catch{errors.push('apiBase 必须是有效 URL')}
 if(release) {
  if(config.devLogin!==false)errors.push('发布时 devLogin 必须为 false');
  if(project.setting?.urlCheck===false)errors.push('发布时必须开启域名校验 urlCheck');
  if(base) {
   if(base.protocol!=='https:')errors.push('发布接口必须使用 HTTPS');
   if(isIP(base.hostname.replace(/^\[|\]$/g,''))||base.hostname==='localhost'||base.hostname.endsWith('.localhost')||base.hostname.endsWith('.example')||!base.hostname.includes('.'))errors.push('发布接口需配置微信后台允许的真实域名，不能使用 IP/本机/占位地址');
   if(base.username||base.password||base.search||base.hash||base.port)errors.push('apiBase 不能包含认证信息、端口、查询或片段');
  }
 }
 return {ok:errors.length===0,errors,files:files.sort(),bytes,appid:project.appid,apiBase:config.apiBase,release};
}

if(process.argv[1]&&path.resolve(process.argv[1])===fileURLToPath(import.meta.url)) {
 const release=process.argv.includes('--release');
 const rootArg=process.argv.indexOf('--project');
 const root=rootArg>=0?process.argv[rootArg+1]:path.resolve(path.dirname(fileURLToPath(import.meta.url)),'..');
 const result=checkProject(root,{release});
 console.log(JSON.stringify(result,null,2));
 if(result.ok)console.log(release?'本地发布配置检查通过；仍需服务器、微信工具与真机验证。':'本地结构检查通过；不代表体验版已经发布。');
 process.exitCode=result.ok?0:1;
}

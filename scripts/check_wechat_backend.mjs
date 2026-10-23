#!/usr/bin/env node
import fs from 'node:fs';
import path from 'node:path';
import {fileURLToPath} from 'node:url';
import {setTimeout as delay} from 'node:timers/promises';

async function boundedBody(response,limit) {
 const reader=response.body?.getReader();
 if(!reader)return new Uint8Array();
 const chunks=[];let size=0;
 try {
  while(true){const {done,value}=await reader.read();if(done)break;size+=value.byteLength;if(size>limit)throw new Error('response exceeds limit');chunks.push(value);}
 } finally {await reader.cancel();}
 return Buffer.concat(chunks,size);
}

export async function checkBackend(apiBase,fetchImpl=fetch,expectedReleaseId) {
 let endpoint='/healthz';
 try {
  const base=new URL(apiBase);
  if(base.protocol!=='https:'||base.username||base.password||base.search||base.hash)throw new Error('invalid URL');
  const request=(route,headers={})=>fetchImpl(new URL(route,base),{headers,redirect:'error',signal:AbortSignal.timeout(10000)});
  const health=await request(endpoint);
  if(health.status!==200||!health.headers.get('content-type')?.includes('application/json')){
   await health.body?.cancel();
   return {ok:false,error:'/healthz 未返回游戏服务的就绪 JSON；请检查云端启动日志和环境变量'};
  }
  const healthBody=await boundedBody(health,2048);
  const state=JSON.parse(healthBody.toString());
  if(state.status!=='ok')return {ok:false,error:'/healthz 未返回游戏服务的就绪 JSON；请检查云端启动日志和环境变量'};
  if(expectedReleaseId&&state.releaseId!==expectedReleaseId)return {ok:false,error:'/healthz 仍在运行旧版本，等待本次 GitHub 推送对应的云托管版本'};
  endpoint='/api/players';
  const players=await request(endpoint);await players.body?.cancel();
  if(players.status!==401)return {ok:false,error:'/api/players 未返回未登录状态；可能仍在运行示例或错误服务'};
  endpoint='/media/audio/1';
  const audio=await request(endpoint,{Range:'bytes=0-3'});
  if(audio.status!==206||!/^"[\x21\x23-\x7e]{1,128}"$/.test(audio.headers.get('etag')||'')||!/^bytes 0-3\/\d+$/.test(audio.headers.get('content-range')||'')||!audio.headers.get('content-type')?.startsWith('audio/')){
   await audio.body?.cancel();
   return {ok:false,error:'/media/audio/1 音频 Range 检查失败'};
  }
  const audioBody=await boundedBody(audio,4);
  if(audioBody.length!==4)return {ok:false,error:'/media/audio/1 音频 Range 检查失败'};
  return {ok:true,checks:['health-json','login-required','audio-range-etag'],deviceTested:false};
 } catch {return {ok:false,error:`${endpoint} 检查失败（连接、证书、重定向或响应不符合要求）`};}
}

if(process.argv[1]&&path.resolve(process.argv[1])===fileURLToPath(import.meta.url)) {
 const args=process.argv.slice(2),option=name=>{const i=args.indexOf(name);return i<0?undefined:args[i+1]};
 try {
  const root=path.resolve(option('--project')||path.join(path.dirname(fileURLToPath(import.meta.url)),'..'));
  const config=JSON.parse(fs.readFileSync(path.join(root,'miniprogram/config.json'),'utf8'));
  const seconds=Number(option('--wait')||0);
  if(!Number.isFinite(seconds)||seconds<0||seconds>900)throw new Error('--wait 必须在 0 到 900 秒之间');
  let expectedReleaseId;
  if(option('--release-id-file')) {
   expectedReleaseId=fs.readFileSync(path.resolve(option('--release-id-file')),'utf8').trim();
   if(!/^[0-9a-f]{32}$/.test(expectedReleaseId))throw new Error('invalid release marker');
  }
  const deadline=Date.now()+seconds*1000;
  let result;
  do {
   result=await checkBackend(config.apiBase,fetch,expectedReleaseId);console.log(JSON.stringify(result));
   if(result.ok||Date.now()>=deadline)break;
   await delay(Math.min(30000,Math.max(0,deadline-Date.now())));
  } while(Date.now()<deadline);
  process.exitCode=result.ok?0:1;
 } catch {console.error('无法读取小程序配置或等待参数无效');process.exitCode=1;}
}

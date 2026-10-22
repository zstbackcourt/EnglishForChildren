import test from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs';
import vm from 'node:vm';
const source=fs.readFileSync(new URL('./app.js',import.meta.url),'utf8');
const extract=(from,to)=>source.slice(source.indexOf(from),source.indexOf(to));
function deferred(){let resolve;const promise=new Promise(r=>{resolve=r});return{promise,resolve}}
function nodes(events){const map=new Map();return {map,get:s=>{if(!map.has(s))map.set(s,{open:true,showModal:()=>events.push('opened')});return map.get(s)}}}
test('pending quiz creation cannot open over another player or lesson',async()=>{
 const request=deferred(),events=[],dom=nodes(events);
 const c={state:{view:'lesson',player:{id:'A'},lesson:{id:1},difficulty:'easy'},loadVersion:1,quizEpoch:0,stop(){},send:()=>request.promise,$:dom.get,renderQuiz:()=>events.push('renderQuiz')};
 vm.createContext(c);vm.runInContext(extract('async function startQuiz()','function modal('),c);
 const pending=c.startQuiz();c.loadVersion++;c.state.player={id:'B'};c.state.lesson={id:3};request.resolve({id:'attempt-A',lessonId:1,questions:[]});await pending;assert.deepEqual(events,[]);
});
test('pending submit cannot redraw after closing and navigating',async()=>{
 const request=deferred(),events=[],dom=nodes(events);const attempt={id:'attempt-A',lessonId:1,difficulty:'easy',questions:Array.from({length:5},()=>({type:'choice',prompt:'Pick',options:['Hello']}))};
 const c={state:{view:'lesson',player:{id:'A'},lesson:{id:1},attempt,answers:Array(5).fill('Hello'),question:4},loadVersion:1,quizEpoch:0,stop(){},guard:f=>f,esc:String,levels:{easy:{name:'Easy',icon:'x'}},modal(){},$$:()=>[],$:dom.get,send:()=>request.promise,api:async path=>{events.push(path);return{id:'B'}},renderLesson:()=>events.push('renderLesson'),renderResult:()=>events.push('renderResult'),toast:()=>{}};
 vm.createContext(c);vm.runInContext(extract('function renderQuiz(){','function renderWords(){'),c);c.renderQuiz();
 const pending=dom.map.get('#quiz-next').onclick();c.loadVersion++;c.quizEpoch++;dom.get('#modal').open=false;c.state.view='map';c.state.player={id:'B'};c.state.lesson={id:3};request.resolve({score:5});await pending;assert.deepEqual(events,[]);
});
test('closing a pending successful submit still refreshes the same players rewards',async()=>{
 const request=deferred(),events=[],dom=nodes(events);const attempt={id:'attempt-A',lessonId:1,difficulty:'easy',questions:Array.from({length:5},()=>({type:'choice',prompt:'Pick',options:['Hello']}))};
 const c={state:{view:'lesson',player:{id:'A',stars:0},lesson:{id:1},attempt,answers:Array(5).fill('Hello'),question:4},loadVersion:1,quizEpoch:0,stop(){},guard:f=>f,esc:String,levels:{easy:{name:'Easy',icon:'x'}},modal(){},$$:()=>[],$:dom.get,send:()=>request.promise,api:async()=>({id:'A',stars:10}),renderLesson:()=>events.push('old lesson'),renderResult:()=>events.push('old result'),render:()=>events.push('current view'),toast:()=>{}};
 vm.createContext(c);vm.runInContext(extract('function renderQuiz(){','function renderWords(){'),c);c.renderQuiz();
 const pending=dom.map.get('#quiz-next').onclick();c.loadVersion++;c.quizEpoch++;dom.get('#modal').open=false;c.state.view='rewards';request.resolve({score:5});await pending;assert.equal(c.state.player.stars,10);assert.deepEqual(events,['current view']);
});

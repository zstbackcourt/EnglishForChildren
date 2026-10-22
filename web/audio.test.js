import test from 'node:test';import assert from 'node:assert/strict';import {lineAt} from './audio.js';
test('subtitle changes at the next cue and last cue remains selected',()=>{const lines=[{start:15.11},{start:16.66},{start:18.26}];assert.equal(lineAt(lines,15.5),0);assert.equal(lineAt(lines,16.66),1);assert.equal(lineAt(lines,99),2);assert.equal(lineAt(lines,0),-1)});

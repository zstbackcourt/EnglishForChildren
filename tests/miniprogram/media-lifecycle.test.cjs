const {test} = require('node:test');
const assert = require('node:assert/strict');
const {AudioPlayer} = require('../../miniprogram/lib/audio');
const {LessonController, ChallengeController} = require('../../miniprogram/lib/controllers');
const tick = () => new Promise(resolve => setImmediate(resolve));
function deferred() { let resolve, reject; const promise = new Promise((yes, no) => { resolve = yes; reject = no; }); promise.catch(() => {}); return {promise, resolve, reject}; }
function fakeWX() {
  const storage = {playerId: 'p1'}, audio = [];
  return {storage, audio, getStorageSync: key => storage[key], setStorageSync(key, value) { storage[key] = value; }, removeStorageSync(key) { delete storage[key]; },
    previewImage(value) { this.preview = value; },
    createInnerAudioContext() {
      const events = {};
      const context = {src: '', paused: true, destroyed: false, currentTime: 0, seeks: [],
        onCanplay: fn => { events.canplay = fn; }, onTimeUpdate: fn => { events.time = fn; }, onEnded: fn => { events.ended = fn; }, onError: fn => { events.error = fn; },
        play() { this.paused = false; }, pause() { this.paused = true; }, stop() { this.paused = true; }, destroy() { this.destroyed = true; }, seek(time) { this.seeks.push(time); },
        emit(name) { events[name]?.(); }};
      audio.push(context); return context;
    }};
}
const lesson = {id: 2, title: 'Hello', audioUrl: '/media/audio/1', scene: '/assets/scenes/lesson-002.png', pageImage: '/assets/pages/page-006.jpg', lines: [{start: 2, end: 4, text: 'Hello'}, {start: 4, end: 8, text: 'Good morning'}]};
const player = {id: 'p1', name: 'Rabbit', avatar: 'rabbit', progress: {}};
const attempt = {id: 'a1', lessonId: 2, difficulty: 'easy', questions: Array.from({length: 5}, (_, id) => ({id, type: 'choice', options: ['Hello'], start: id + 1, end: id + 2}))};
function apiFixture(asset, submit = async () => ({score: 5})) {
  return {asset, async request(path, method) {
    if (path === '/api/players') return [player];
    if (path.startsWith('/api/lessons/')) return {...lesson, id: Number(path.split('/').at(-1))};
    if (path.endsWith('/progress')) return {saved: true};
    if (path.endsWith('/attempts') && method === 'POST') return attempt;
    if (path.endsWith('/attempts/a1')) return submit();
    throw new Error('Unexpected API request: ' + path);
  }};
}

test('audio waits for the resolved local file before creating a playable context', async () => {
  const wx = fakeWX(), pending = deferred(), states = [];
  const audio = new AudioPlayer(wx, state => states.push(state), path => { assert.equal(path, '/media/audio/1'); return pending.promise; });
  audio.play('/media/audio/1', lesson.lines, 1, false);
  assert.equal(wx.audio.length, 0);
  assert.equal(states.at(-1).audioLoading, true);
  pending.resolve('/user/cache/audio.mp3'); await tick();
  assert.equal(wx.audio[0].src, '/user/cache/audio.mp3');
  wx.audio[0].emit('canplay');
  assert.deepEqual(wx.audio[0].seeks, [4]);
  assert.equal(wx.audio[0].paused, false);
  assert.equal(states.at(-1).audioLoading, false);
});
for (const action of ['stop', 'destroy']) test('audio ' + action + ' prevents a pending resolution from starting playback', async () => {
  const wx = fakeWX(), pending = deferred(), states = [];
  const audio = new AudioPlayer(wx, state => states.push(state), () => pending.promise);
  audio.play('/media/audio/1', lesson.lines, 0, false); audio[action]();
  const count = states.length;
  pending.resolve('/user/cache/audio.mp3'); await tick();
  assert.equal(wx.audio.length, 0);
  assert.equal(states.length, count);
  assert.equal(states.at(-1).audioLoading, false);
});
test('a newer audio request wins when the old file resolves last', async () => {
  const wx = fakeWX(), first = deferred(), second = deferred(), states = [];
  const audio = new AudioPlayer(wx, state => states.push(state), path => path === '/first' ? first.promise : second.promise);
  audio.play('/first', lesson.lines, 0, false); audio.play('/second', lesson.lines, 1, false);
  second.resolve('/user/second.mp3'); await tick(); first.resolve('/user/first.mp3'); await tick();
  assert.equal(wx.audio.length, 1); assert.equal(wx.audio[0].src, '/user/second.mp3');
  wx.audio[0].emit('canplay'); assert.deepEqual(wx.audio[0].seeks, [4]); assert.equal(states.at(-1).cursor, 1);
});
test('audio retry resolves the original source again after a rejected download', async () => {
  const wx = fakeWX(), first = deferred(), second = deferred(), sources = [], states = [];
  const audio = new AudioPlayer(wx, state => states.push(state), path => { sources.push(path); return sources.length === 1 ? first.promise : second.promise; });
  audio.play('/media/audio/1', lesson.lines, 0, false); first.reject(new Error('download failed')); await tick();
  assert.equal(wx.audio.length, 0); assert.match(states.at(-1).audioError, /重试/); assert.equal(states.at(-1).audioLoading, false);
  audio.retry(); second.resolve('/user/retried.mp3'); await tick();
  assert.deepEqual(sources, ['/media/audio/1', '/media/audio/1']); assert.equal(wx.audio[0].src, '/user/retried.mp3');
});
test('synchronous resolver failures are recoverable without leaving audio loading', () => {
  const wx = fakeWX(), states = []; let failed = true;
  const audio = new AudioPlayer(wx, state => states.push(state), () => { if (failed) throw new Error('offline'); return '/user/audio.mp3'; });
  audio.play('/media/audio/1', lesson.lines, 0, false);
  assert.match(states.at(-1).audioError, /重试/); assert.equal(states.at(-1).audioLoading, false);
  failed = false; audio.retry(); assert.equal(wx.audio[0].src, '/user/audio.mp3');
});
test('a role becoming inactive blocks both file completion and late canplay', async () => {
  const wx = fakeWX(), pending = deferred(), states = []; let active = true;
  const audio = new AudioPlayer(wx, state => states.push(state), () => pending.promise, () => active);
  audio.play('/media/audio/1', lesson.lines, 0, false); active = false; const count = states.length;
  pending.resolve('/user/audio.mp3'); await tick(); assert.equal(wx.audio.length, 0); assert.equal(states.length, count);
  active = true; audio.retry(); await tick(); active = false; wx.audio[0].emit('canplay');
  assert.equal(wx.audio[0].paused, true); assert.deepEqual(wx.audio[0].seeks, []);
});

for (const action of ['pause', 'hide', 'role']) test('lesson ' + action + ' cancels a pending cloud audio request', async () => {
  const wx = fakeWX(), pending = deferred();
  const controller = new LessonController(apiFixture(path => path.startsWith('/media/') ? pending.promise : '/user/image.png'), wx);
  await controller.load({id: 2}); controller.playLine(0);
  if (action === 'role') wx.storage.playerId = 'p2'; else controller[action]();
  const before = JSON.stringify(controller.data);
  pending.resolve('/user/audio.mp3'); await tick();
  assert.equal(wx.audio.length, 0); assert.equal(JSON.stringify(controller.data), before);
  if (action === 'pause') assert.equal(controller.data.audioLoading, false);
});
test('lesson loads its text while scene and page images resolve independently', async () => {
  const wx = fakeWX(), scene = deferred(), page = deferred();
  const controller = new LessonController(apiFixture(path => path === lesson.scene ? scene.promise : page.promise), wx);
  await controller.load({id: 2});
  assert.equal(controller.data.loading, false); assert.equal(controller.data.lesson.title, 'Hello');
  assert.equal(controller.data.sceneUrl, ''); assert.equal(controller.data.pageUrl, ''); assert.equal(controller.data.imageLoading, true);
  scene.resolve('/user/scene.png'); await tick();
  assert.equal(controller.data.sceneUrl, '/user/scene.png'); assert.equal(controller.data.pageUrl, '');
  controller.preview(); assert.deepEqual(wx.preview.urls, ['/user/scene.png']);
  page.resolve('/user/page.jpg'); await tick();
  assert.equal(controller.data.pageUrl, '/user/page.jpg'); assert.equal(controller.data.imageLoading, false);
});
for (const action of ['hide', 'role']) test('lesson images cannot update after ' + action, async () => {
  const wx = fakeWX(), pending = deferred();
  const controller = new LessonController(apiFixture(() => pending.promise), wx);
  await controller.load({id: 2});
  if (action === 'role') wx.storage.playerId = 'p2'; else controller.hide();
  const before = JSON.stringify(controller.data); pending.resolve('/user/old.png'); await tick();
  assert.equal(controller.data.sceneUrl, ''); assert.equal(controller.data.pageUrl, ''); assert.equal(JSON.stringify(controller.data), before);
  controller.preview(); assert.equal(wx.preview, undefined);
});
test('a new lesson clears previous image URLs and ignores late previous image results', async () => {
  const wx = fakeWX(), oldPage = deferred(), nextScene = deferred(), nextPage = deferred(); let loads = 0;
  const controller = new LessonController(apiFixture(path => {
    if (++loads === 1) return '/user/old-scene.png';
    if (loads === 2) return oldPage.promise;
    return path === lesson.scene ? nextScene.promise : nextPage.promise;
  }), wx);
  await controller.load({id: 2}); assert.equal(controller.data.sceneUrl, '/user/old-scene.png');
  const loading = controller.load({id: 3}); assert.equal(controller.data.sceneUrl, ''); assert.equal(controller.data.pageUrl, ''); await loading;
  oldPage.resolve('/user/old-page.jpg'); await tick(); assert.equal(controller.data.pageUrl, '');
  nextScene.resolve('/user/new-scene.png'); nextPage.resolve('/user/new-page.jpg'); await tick();
  assert.equal(controller.data.sceneUrl, '/user/new-scene.png'); assert.equal(controller.data.pageUrl, '/user/new-page.jpg');
});
test('failed images preserve lesson text and retry only the failed image', async () => {
  const wx = fakeWX(), failed = deferred(), retry = deferred(); let scenes = 0, pages = 0;
  const controller = new LessonController(apiFixture(path => {
    if (path === lesson.scene) return ++scenes === 1 ? failed.promise : retry.promise;
    pages++; return '/user/page.jpg';
  }), wx);
  await controller.load({id: 2}); failed.reject(new Error('offline')); await tick();
  assert.equal(controller.data.error, ''); assert.equal(controller.data.lesson.title, 'Hello'); assert.match(controller.data.imageError, /重试/);
  controller.retryImages(); retry.resolve('/user/scene.png'); await tick();
  assert.equal(controller.data.sceneUrl, '/user/scene.png'); assert.equal(controller.data.pageUrl, '/user/page.jpg'); assert.equal(controller.data.imageError, ''); assert.equal(pages, 1);
});
for (const action of ['next', 'hide', 'role', 'submit']) test('challenge ' + action + ' cannot start a late audio hint', async () => {
  const wx = fakeWX(), pending = deferred(), submitting = deferred();
  const controller = new ChallengeController(apiFixture(() => pending.promise, () => submitting.promise), wx);
  await controller.load({id: 2, difficulty: 'easy'});
  if (action === 'submit') for (let i = 0; i < 4; i++) { controller.answer('Hello'); controller.next(); }
  controller.hint(); controller.answer('Hello');
  let submitted;
  if (action === 'role') wx.storage.playerId = 'p2'; else if (action === 'submit') submitted = controller.submit(); else controller[action]();
  const before = JSON.stringify(controller.data); pending.resolve('/user/audio.mp3'); await tick();
  assert.equal(wx.audio.length, 0); assert.equal(JSON.stringify(controller.data), before);
  if (action === 'next' || action === 'submit') assert.equal(controller.data.audioLoading, false);
  if (submitted) { submitting.resolve({score: 5}); await submitted; }
});

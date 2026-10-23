const {test} = require('node:test');
const assert = require('node:assert/strict');
const {AudioPlayer} = require('../../miniprogram/lib/audio');
const {ChallengeController} = require('../../miniprogram/lib/controllers');
const {lessons} = require('../../content/book1.json');
const lesson = lessons.find(item => item.id === 1);

function audioFixture() {
  const contexts = [], state = {}, updates = [];
  const wx = {
    createInnerAudioContext() {
      const handlers = {};
      const context = {currentTime: 0, paused: true, destroyed: false, seeks: [],
        onCanplay: fn => { handlers.canplay = fn; }, onTimeUpdate: fn => { handlers.time = fn; },
        onEnded: fn => { handlers.ended = fn; }, onError: fn => { handlers.error = fn; },
        seek(time) { this.seeks.push(time); this.currentTime = time; }, play() { this.paused = false; },
        pause() { this.paused = true; }, stop() { this.paused = true; }, destroy() { this.destroyed = true; },
        emit(event, time) { if (time !== undefined) this.currentTime = time; handlers[event]?.(); }};
      contexts.push(context); return context;
    }
  };
  const player = new AudioPlayer(wx, value => { updates.push(value); Object.assign(state, value); });
  return {wx, contexts, state, updates, player};
}

test('real lesson 1 plays continuously past the first time update and tracks subtitles until native end', () => {
  const {player, contexts, state} = audioFixture();
  player.play(lesson.audioUrl, lesson.lines, 0, true);
  const context = contexts[0]; context.emit('canplay');
  context.emit('time', 15.2);
  assert.equal(context.paused, false, 'the final end=0 is an open boundary, not zero seconds');
  assert.equal(state.playing, true);
  context.emit('time', 16.8); assert.equal(state.cursor, 1); assert.equal(context.paused, false);
  context.emit('time', 18.4); assert.equal(state.cursor, 2); assert.equal(context.paused, false);
  context.emit('time', 30); assert.equal(state.cursor, 6); assert.equal(context.paused, false);
  context.emit('ended'); assert.equal(state.playing, false); assert.equal(context.paused, true);
});

test('the real final sentence plays to native end when selected on its own', () => {
  const {player, contexts, state} = audioFixture();
  player.play(lesson.audioUrl, lesson.lines, lesson.lines.length - 1, false);
  const context = contexts[0]; context.emit('canplay');
  assert.deepEqual(context.seeks, [29.49]);
  context.emit('time', 30); assert.equal(context.paused, false); assert.equal(state.playing, true);
  context.emit('ended'); assert.equal(context.paused, true); assert.equal(state.playing, false);
});

test('a challenge hint for the real final sentence preserves the open audio boundary', async () => {
  const fixture = audioFixture(), storage = {playerId: 'p1'};
  const last = lesson.lines.at(-1);
  Object.assign(fixture.wx, {getStorageSync: key => storage[key], setStorageSync(key, value) { storage[key] = value; }, removeStorageSync(key) { delete storage[key]; }});
  const api = {asset: path => path, async request(path) {
    if (path === '/api/players') return [{id: 'p1', name: 'Test', progress: {}}];
    if (path === '/api/lessons/1') return lesson;
    if (path === '/api/players/p1/attempts') return {id: 'a1', difficulty: 'easy', questions: Array.from({length: 5}, (_, id) => ({id, type: 'choice', options: ['Thank you very much.'], start: last.start, end: last.end}))};
    throw new Error('unexpected fixture request');
  }};
  const controller = new ChallengeController(api, fixture.wx);
  await controller.load({id: 1, difficulty: 'easy'}); controller.hint();
  const context = fixture.contexts[0]; context.emit('canplay'); context.emit('time', 30);
  assert.equal(context.paused, false); assert.equal(controller.data.playing, true);
  context.emit('ended'); assert.equal(controller.data.playing, false);
});

for (const [name, end] of [['zero', 0], ['missing', undefined], ['negative', -1], ['equal to start', 5], ['before start', 4], ['infinity', Infinity], ['negative infinity', -Infinity], ['NaN', NaN], ['numeric text', '7']]) {
  test('an end that is ' + name + ' waits for native end instead of stopping on time updates', () => {
    const {player, contexts, state} = audioFixture();
    player.play('/media/audio/1', [{start: 5, end}], 0, false);
    const context = contexts[0]; context.emit('canplay'); context.emit('time', 10);
    assert.equal(context.paused, false); assert.equal(state.playing, true);
    context.emit('ended'); assert.equal(state.playing, false); assert.equal(context.paused, true);
  });
}

for (const continuous of [false, true]) test('a valid finite end still stops ' + (continuous ? 'continuous' : 'single sentence') + ' playback at its boundary', () => {
  const {player, contexts, state} = audioFixture();
  player.play('/media/audio/1', [{start: 2, end: 4}, {start: 4, end: 7}], 0, continuous);
  const context = contexts[0]; context.emit('canplay'); context.emit('time', 3.9);
  assert.equal(context.paused, false);
  if (continuous) { context.emit('time', 4.1); assert.equal(context.paused, false); assert.equal(state.cursor, 1); }
  context.emit('time', continuous ? 7 : 4); assert.equal(context.paused, true); assert.equal(state.playing, false);
});

for (const action of ['stop', 'destroy']) test(action + ' still cancels open-ended playback and ignores its late events', () => {
  const {player, contexts, state, updates} = audioFixture();
  player.play(lesson.audioUrl, lesson.lines, 0, true);
  const context = contexts[0]; context.emit('canplay'); player[action]();
  const before = updates.length;
  context.emit('time', 30); context.emit('ended'); context.emit('canplay');
  assert.equal(updates.length, before); assert.equal(state.playing, false); assert.equal(context.destroyed, true); assert.equal(context.paused, true);
});

test('an old open-ended track cannot end its replacement', () => {
  const {player, contexts, state, updates} = audioFixture();
  player.play(lesson.audioUrl, lesson.lines, 0, true); contexts[0].emit('canplay');
  player.play(lesson.audioUrl, lesson.lines, 2, true); contexts[1].emit('canplay');
  const before = updates.length; contexts[0].emit('time', 30); contexts[0].emit('ended');
  assert.equal(updates.length, before); assert.equal(contexts[1].paused, false); assert.equal(state.playing, true); assert.equal(state.cursor, 2);
});

/* One context per playback prevents a late canplay/seek from an old track. */
class AudioPlayer {
  constructor(wx, update, resolveSource = path => path, isActive = () => true) {
    this.wx = wx; this.update = update; this.resolveSource = resolveSource; this.isActive = isActive;
    this.generation = 0; this.context = null; this.last = null;
  }
  stop() {
    this.generation++;
    if (this.context) { this.context.stop(); this.context.destroy(); this.context = null; }
    if (this.isActive()) this.update({playing: false, audioLoading: false});
  }
  destroy() { this.stop(); this.last = null; }
  play(path, lines, index, continuous) {
    this.stop();
    if (!this.isActive() || !path || !lines || !lines[index]) return;
    this.last = [path, lines, index, continuous];
    const generation = this.generation;
    const active = () => this.generation === generation && this.isActive();
    const fail = () => { if (active()) this.update({playing: false, audioLoading: false, audioError: '音频暂时无法播放，请点重试'}); };
    const resolved = url => {
      if (!active()) return;
      if (typeof url !== 'string' || !url) { fail(); return; }
      this.start(url, lines, index, continuous, generation);
    };
    this.update({playing: false, audioLoading: true, cursor: index, audioError: ''});
    try {
      const source = this.resolveSource(path);
      // Keep HTTP development sources synchronous; cloud files resolve later.
      if (source && typeof source.then === 'function') return Promise.resolve(source).then(resolved).catch(fail);
      resolved(source);
    } catch (e) { fail(); }
  }
  start(url, lines, index, continuous, generation) {
    if (this.generation !== generation || !this.isActive()) return;
    const context = this.wx.createInnerAudioContext();
    this.context = context;
    let ready = false;
    let cursor = index;
    let stopped = false;
    const active = () => this.generation === generation && this.context === context && this.isActive();
    const emit = data => { if (active()) this.update(data); };
    const finish = () => { if (active()) { stopped = true; context.pause(); emit({playing: false, audioLoading: false}); } };
    context.autoplay = false;
    context.obeyMuteSwitch = false;
    context.onCanplay(() => {
      if (!active() || ready || stopped) return;
      ready = true;
      context.seek(lines[index].start);
      context.play();
      emit({playing: true, audioLoading: false, cursor: index, audioError: ''});
    });
    context.onTimeUpdate(() => {
      if (!active() || !ready || stopped) return;
      const time = context.currentTime;
      const boundary = continuous ? lines[lines.length - 1] : lines[index];
      // A final end of zero means play to the native end of the audio file.
      if (Number.isFinite(boundary.end) && boundary.end > boundary.start && time >= boundary.end) { finish(); return; }
      if (continuous) {
        let next = index;
        for (let i = index; i < lines.length; i++) { if (time >= lines[i].start) next = i; }
        if (next !== cursor) { cursor = next; emit({cursor}); }
      }
    });
    context.onEnded(finish);
    context.onError(() => { if (active()) { stopped = true; context.pause(); emit({playing: false, audioLoading: false, audioError: '音频暂时无法播放，请点重试'}); } });
    context.src = url;
  }
  retry() { if (this.last) this.play(...this.last); }
}
module.exports = {AudioPlayer};

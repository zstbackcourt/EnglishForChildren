/* One context per playback prevents a late canplay/seek from an old track. */
class AudioPlayer {
  constructor(wx, update) { this.wx = wx; this.update = update; this.generation = 0; this.context = null; this.last = null; }
  stop() {
    this.generation++;
    if (this.context) { this.context.stop(); this.context.destroy(); this.context = null; }
  }
  destroy() { this.stop(); this.last = null; }
  play(url, lines, index, continuous) {
    this.stop();
    if (!url || !lines || !lines[index]) return;
    this.last = [url, lines, index, continuous];
    const generation = this.generation;
    const context = this.wx.createInnerAudioContext();
    this.context = context;
    let ready = false;
    let cursor = index;
    let stopped = false;
    const active = () => this.generation === generation && this.context === context;
    const emit = data => { if (active()) this.update(data); };
    const finish = () => { if (active()) { stopped = true; context.pause(); emit({playing: false}); } };
    context.autoplay = false;
    context.obeyMuteSwitch = false;
    context.onCanplay(() => {
      if (!active() || ready || stopped) return;
      ready = true;
      context.seek(lines[index].start);
      context.play();
      emit({playing: true, cursor: index, audioError: ''});
    });
    context.onTimeUpdate(() => {
      if (!active() || !ready || stopped) return;
      const time = context.currentTime;
      if (time >= (continuous ? lines[lines.length - 1].end : lines[index].end)) { finish(); return; }
      if (continuous) {
        let next = index;
        for (let i = index; i < lines.length; i++) { if (time >= lines[i].start) next = i; }
        if (next !== cursor) { cursor = next; emit({cursor}); }
      }
    });
    context.onEnded(finish);
    context.onError(() => { if (active()) { stopped = true; context.pause(); emit({playing: false, audioError: '音频暂时无法播放，请点重试'}); } });
    emit({playing: false, cursor: index, audioError: ''});
    context.src = url;
  }
  retry() { if (this.last) this.play(...this.last); }
}
module.exports = {AudioPlayer};

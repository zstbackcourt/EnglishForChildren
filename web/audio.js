export function lineAt(lines, time) {
  let index = -1;
  for (let i = 0; i < lines.length && lines[i].start <= time; i++) index = i;
  return index;
}
export class Voice {
  constructor(onTime, onState, onError) {
    this.generation = 0; this.audio = new Audio(); this.audio.preload = 'metadata'; this.boundary = 0;
    this.audio.addEventListener('timeupdate', () => {
      if (this.boundary && this.audio.currentTime >= this.boundary) this.pause();
      onTime(this.audio.currentTime);
    });
    for (const event of ['play', 'pause', 'ended']) this.audio.addEventListener(event, () => onState(!this.audio.paused));
    this.audio.addEventListener('error', () => onError('音频加载失败，请检查服务后重试。'));
  }
  setSource(url) { this.pause(); if (this.audio.getAttribute('src') !== url) this.audio.src = url; }
  async play(start, end = 0) {
    const generation = ++this.generation;
    this.boundary = end;
    if (this.audio.readyState === 0) await new Promise((resolve, reject) => {
      const ready = () => { cleanup(); resolve(); }; const error = () => { cleanup(); reject(new Error('音频加载失败')); };
      const timer = setTimeout(error, 10000);
      const cleanup = () => { clearTimeout(timer); this.audio.removeEventListener('loadedmetadata', ready); this.audio.removeEventListener('error', error); };
      this.audio.addEventListener('loadedmetadata', ready); this.audio.addEventListener('error', error);
    });
    if (generation !== this.generation) return;
    this.audio.currentTime = start; await this.audio.play();
  }
  pause() { this.generation++; this.audio.pause(); }
  rate(value) { this.audio.playbackRate = value; }
}

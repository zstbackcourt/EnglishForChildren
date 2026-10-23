const {AudioPlayer} = require('./audio');
const difficulties = {easy: '小小耳朵', medium: '句子魔法', hard: '单词勇士'};
const avatars = {fox: '🦊', cat: '🐱', rabbit: '🐰', bear: '🐻'};
function playerPath(id) { return '/api/players/' + encodeURIComponent(id); }
class Controller {
  constructor(api, wx, update = () => {}) {
    this.api = api; this.wx = wx; this.update = update; this.generation = 0; this.active = false;
    this.data = {loading: false, error: '', player: null, avatars, difficulties};
  }
  set(value) { this.data = Object.assign({}, this.data, value); this.update(value); }
  begin() { this.active = true; const g = ++this.generation; this.set({loading: true, error: ''}); return g; }
  live(g, playerId) { return this.active && this.generation === g && (!playerId || this.wx.getStorageSync('playerId') === playerId); }
  hide() { this.active = false; this.generation++; if (this.audio) this.audio.destroy(); }
  async family(g) {
    const players = await this.api.request('/api/players');
    if (!this.live(g)) return null;
    const player = players.find(p => p.id === this.wx.getStorageSync('playerId')) || players[0] || null;
    if (player) this.wx.setStorageSync('playerId', player.id); else this.wx.removeStorageSync('playerId');
    this.set({players, player});
    return player;
  }
  fail(g, e, playerId) { if (this.live(g, playerId)) this.set({loading: false, busy: false, error: e.message || '操作失败，请重试'}); }
}
class MapController extends Controller {
  constructor(...args) { super(...args); this.set({chapters: [], resumeId: 1}); }
  async load() {
    const g = this.begin();
    try {
      const [lessons, player] = await Promise.all([this.api.request('/api/lessons'), this.family(g)]);
      if (!this.live(g)) return;
      const chapters = Array.from({length: 12}, (_,i) => ({id: i + 1, title: ['你好，世界','认识朋友','温暖的家','快乐校园','小小探险家','美味时光','多彩生活','一起出发','奇妙发现','成长的魔法','勇敢表达','星光毕业'][i], lessons: lessons.slice(i * 12, i * 12 + 12).map(l => {
        const p = player && player.progress && player.progress[l.id];
        return {id: l.id, title: l.title, kind: l.kind, trophies: p ? Object.values(p.levels || {}).filter(x => x.trophy).length : 0};
      })}));
      this.set({loading: false, chapters, resumeId: player ? player.lastLesson : 1});
    } catch (e) { this.fail(g, e); }
  }
}
class PlayersController extends Controller {
  constructor(...args) { super(...args); this.set({players: [], busy: false}); }
  async load() { const g = this.begin(); try { await this.family(g); if (this.live(g)) this.set({loading: false}); } catch (e) { this.fail(g,e); } }
  select(id) {
    const player = this.data.players.find(p => p.id === id);
    if (!this.active || !player) return;
    this.wx.setStorageSync('playerId', id); this.set({player});
  }
  async create(name, avatar) {
    if (!this.active || this.data.busy) return;
    name = String(name || '').trim();
    if (!name || name.length > 16 || !avatars[avatar]) { this.set({error: '请输入 1–16 字昵称，并选择一个伙伴'}); return; }
    const g = this.generation;
    this.set({busy: true, error: ''});
    try {
      const player = await this.api.request('/api/players', 'POST', {name, avatar});
      if (!this.live(g)) return;
      this.wx.setStorageSync('playerId', player.id);
      this.set({players: [...this.data.players, player], player, busy: false});
    } catch (e) { this.fail(g,e); }
  }
}
class LessonController extends Controller {
  constructor(...args) { super(...args); this.set({lesson: null, cursor: 0, difficulty: 'easy', playing: false, audioError: '', progressError: '', showTranslation: true}); }
  async load(options) {
    if (this.audio) this.audio.destroy();
    const g = this.begin();
    const id = Number(options.id);
    if (!Number.isInteger(id) || id < 1 || id > 144) { this.fail(g,new Error('课号无效')); return; }
    try {
      const [lesson, player] = await Promise.all([this.api.request('/api/lessons/' + id), this.family(g)]);
      if (!this.live(g, player && player.id)) return;
      const progress = player && player.progress && player.progress[id];
      const cursor = progress ? Math.min(Math.max(progress.cursor,0),lesson.lines.length - 1) : 0;
      const difficulty = progress && difficulties[progress.lastDifficulty] ? progress.lastDifficulty : 'easy';
      this.set({showTranslation: this.wx.getStorageSync('showTranslation') !== false, loading: false, lesson, cursor, difficulty, playing: false, audioError: '', progressError: '', sceneUrl: this.api.asset(lesson.scene), pageUrl: this.api.asset(lesson.pageImage), scrollTo: 'line-' + cursor});
      this.audio = new AudioPlayer(this.wx, state => {
        if (!this.live(g, player && player.id)) return;
        this.set(Object.assign({},state,state.cursor === undefined ? {} : {scrollTo: 'line-' + state.cursor}));
        if (state.cursor !== undefined && player) this.saveProgress();
      });
    } catch (e) { this.fail(g,e); }
  }
  playLine(index, continuous = false) {
    index = Number(index);
    if (!this.active || !this.audio || !this.data.lesson || !this.data.lesson.lines[index]) return;
    if (this.data.player && this.wx.getStorageSync('playerId') !== this.data.player.id) { this.hide(); return; }
    this.audio.play(this.api.asset(this.data.lesson.audioUrl), this.data.lesson.lines, index, continuous);
  }
  pause() { if (this.audio) this.audio.stop(); this.set({playing: false}); }
  retryAudio() { if (this.active && this.audio) this.audio.retry(); }
  setDifficulty(difficulty) { if (difficulties[difficulty]) { this.set({difficulty}); this.saveProgress(); } }
  async saveProgress() {
    const {player, lesson, cursor, difficulty} = this.data;
    const g = this.generation;
    if (!player || !lesson || !this.live(g,player.id)) return;
    // A new visible lifecycle also waits for older writes to avoid server races.
    const body = {lessonId: lesson.id, cursor, difficulty};
    this.progressTail = (this.progressTail || Promise.resolve()).then(async () => {
      if (!this.live(g,player.id)) return;
      try {
        await this.api.request(playerPath(player.id) + '/progress', 'PUT', body);
        if (this.live(g,player.id)) this.set({progressError: ''});
      } catch (e) {
        if (this.live(g,player.id)) this.set({progressError: '进度保存失败，点此重试'});
      }
    });
    return this.progressTail;
  }

  preview() { const urls = [this.data.sceneUrl,this.data.pageUrl].filter(Boolean); if(this.active && urls.length) this.wx.previewImage({current: urls[0],urls}); }
  share() { const l = this.data.lesson; return {title: l ? '第 ' + l.id + ' 课 · ' + l.title : '英语星光冒险', path: '/pages/lesson/lesson?id=' + (l ? l.id : 1)}; }
  toggleTranslation() { this.set({showTranslation: !this.data.showTranslation}); }
}
class ChallengeController extends Controller {
  constructor(...args) { super(...args); this.set({attempt: null, lesson: null, question: null, index: 0, answer: '', orderSelected: [], optionItems: [], busy: false, result: null, audioError: '', playing: false}); this.answers = []; }
  async load(options) {
    if (this.audio) this.audio.destroy();
    const g = this.begin();
    const id = Number(options.id), difficulty = options.difficulty;
    this.set({attempt: null, result: null, question: null, busy: false}); this.answers = [];
    if (!Number.isInteger(id) || id < 1 || id > 144 || !difficulties[difficulty]) { this.fail(g,new Error('课号或难度无效')); return; }
    try {
      const [lesson,player] = await Promise.all([this.api.request('/api/lessons/' + id),this.family(g)]);
      if (!this.live(g)) return;
      if (!player) throw new Error('先创建一个冒险角色，再来挑战吧');
      if (!this.live(g,player.id)) return;
      const attempt = await this.api.request(playerPath(player.id) + '/attempts','POST',{lessonId:id,difficulty});
      if (!this.live(g,player.id)) return;
      this.set({attempt, lesson, loading: false});
      this.audio = new AudioPlayer(this.wx, s => { if(this.live(g,player.id)) this.set(s); });
      this.showQuestion(0);
    } catch (e) { this.fail(g,e); }
  }
  showQuestion(index) {
    if (this.audio) this.audio.destroy();
    const question = this.data.attempt.questions[index];
    this.set({question, index, answer: this.answers[index] || '', orderSelected: [], optionItems: (question.options || []).map((text,id)=>({id,text,selected:false})), audioError: '', playing:false});
  }
  answer(value) { if (this.active && !this.data.busy && !this.data.result) this.set({answer: String(value), error: ''}); }
  order(index) {
    if (!this.active || this.data.busy || this.data.result || this.data.question.type !== 'order') return;
    index = Number(index);
    if (!this.data.optionItems[index]) return;
    const selected = this.data.orderSelected.includes(index) ? this.data.orderSelected.filter(i=>i!==index) : [...this.data.orderSelected,index];
    this.set({orderSelected:selected, answer: selected.map(i=>this.data.question.options[i]).join(' '),optionItems:this.data.optionItems.map(o=>Object.assign({},o,{selected:selected.includes(o.id)}))});
  }
  next() {
    if (!this.active || !this.data.question || !this.data.answer.trim() || this.data.busy || this.data.result) return;
    this.answers[this.data.index] = this.data.answer.trim();
    if (this.data.index < this.data.attempt.questions.length - 1) this.showQuestion(this.data.index + 1);
  }
  hint() {
    if (!this.active || !this.audio || !this.data.question || this.data.busy || this.data.result) return;
    const q=this.data.question;
    this.audio.play(this.api.asset(this.data.lesson.audioUrl),[{start:q.start,end:q.end}],0,false);
  }
  retryAudio() { if (this.active && this.audio) this.audio.retry(); }
  async submit() {
    const {attempt,player} = this.data;
    const g=this.generation;
    if (!attempt || !player || !this.live(g,player.id) || this.data.busy || this.data.result) return;
    this.answers[this.data.index] = this.data.answer.trim();
    if (this.answers.length !== 5 || this.answers.some(a=>!a)) { this.set({error:'请完成全部 5 道题'}); return; }
    this.set({busy:true,error:''}); if(this.audio) this.audio.destroy();
    try {
      const result = await this.api.request(playerPath(player.id) + '/attempts/' + encodeURIComponent(attempt.id),'POST',{answers:this.answers.slice()});
      if(this.live(g,player.id)) this.set({result,busy:false});
    } catch (e) { this.fail(g,e,player.id); }
  }
}
class RewardsController extends Controller {
  constructor(...args) { super(...args); this.set({stars:0,trophies:[]}); }
  async load() {
    const g=this.begin();
    try {
      const player=await this.family(g); if(!this.live(g))return;
      const trophies=[];
      if(player) Object.keys(player.progress || {}).forEach(id=>Object.keys(player.progress[id].levels || {}).forEach(d=>{const level=player.progress[id].levels[d];if(level.trophy)trophies.push({lessonId:Number(id),difficulty:d,label:difficulties[d],bestScore:level.bestScore});}));
      this.set({loading:false,stars:player?player.stars:0,trophies});
    } catch(e){this.fail(g,e);}
  }
}
module.exports={MapController,PlayersController,LessonController,ChallengeController,RewardsController};

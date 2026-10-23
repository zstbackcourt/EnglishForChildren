const {nativePage} = require('../../lib/page');
const {ChallengeController} = require('../../lib/controllers');
Page(nativePage(ChallengeController,{
  choose(e) { this.controller.answer(e.currentTarget.dataset.value); },
  typeAnswer(e) { this.controller.answer(e.detail.value); },
  order(e) { this.controller.order(e.currentTarget.dataset.index); },
  next() { this.controller.next(); },
  hint() { this.controller.hint(); },
  retryAudio() { this.controller.retryAudio(); },
  submit() { return this.controller.submit(); },
  retry() { return this.data.attempt ? this.controller.submit() : this.controller.load(this.options); },
  again() { return this.controller.load(this.options); },
  rewards() { wx.switchTab({url:'/pages/rewards/rewards'}); }
}));

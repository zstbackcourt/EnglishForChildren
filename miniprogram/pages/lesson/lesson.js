const {nativePage} = require('../../lib/page');
const {LessonController} = require('../../lib/controllers');
Page(nativePage(LessonController,{
  playLine(e) { this.controller.playLine(e.currentTarget.dataset.index); },
  playAll() { this.controller.playLine(this.data.cursor,true); },
  pause() { this.controller.pause(); },
  retryAudio() { this.controller.retryAudio(); },
  saveProgress() { return this.controller.saveProgress(); },
  preview() { this.controller.preview(); },
  toggleTranslation() { this.controller.toggleTranslation(); },
  difficulty(e) { this.controller.setDifficulty(e.currentTarget.dataset.id); },
  challenge() { if(!this.data.player) { this.players(); return; } wx.navigateTo({url:'/pages/challenge/challenge?id='+this.data.lesson.id+'&difficulty='+this.data.difficulty}); },
  onShareAppMessage() { return this.controller.share(); }
}));

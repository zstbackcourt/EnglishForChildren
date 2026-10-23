const {nativePage} = require('../../lib/page');
const {RewardsController} = require('../../lib/controllers');
Page(nativePage(RewardsController,{
  openLesson(e) { wx.navigateTo({url:'/pages/lesson/lesson?id='+Number(e.currentTarget.dataset.id)}); }
}));

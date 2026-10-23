const {nativePage} = require('../../lib/page');
const {MapController} = require('../../lib/controllers');
Page(nativePage(MapController,{
  openLesson(e) { const id=Number(e.currentTarget.dataset.id); if(id>=1 && id<=144) wx.navigateTo({url:'/pages/lesson/lesson?id='+id}); },
  resume() { this.openLesson({currentTarget:{dataset:{id:this.data.resumeId}}}); }
}));

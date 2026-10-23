/* Native lifecycle adapter; all application decisions live in controllers. */
function nativePage(Controller, methods) {
  return Object.assign({
    data: {},
    onLoad(options) {
      this.options = options || {};
      this.controller = new Controller(getApp().api, wx, data => this.setData(data));
      this.setData(this.controller.data);
    },
    onShow() { return this.controller.load(this.options); },
    onHide() { this.controller.hide(); },
    onUnload() { this.controller.hide(); },
    retry() { return this.controller.load(this.options); },
    players() { wx.switchTab({url:'/pages/players/players'}); },
    map() { wx.switchTab({url:'/pages/index/index'}); }
  },methods);
}
module.exports = {nativePage};

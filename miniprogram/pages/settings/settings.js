const config = require('../../config.js');
Page({
  data:{showTranslation:true,development:config.devLogin},
  onLoad() {},
  onShow() { this.setData({showTranslation:wx.getStorageSync('showTranslation') !== false}); },
  translationChanged(e) { const showTranslation=!!e.detail.value; wx.setStorageSync('showTranslation',showTranslation); this.setData({showTranslation}); },
  players() { wx.switchTab({url:'/pages/players/players'}); }
});

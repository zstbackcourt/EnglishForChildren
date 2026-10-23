const {nativePage} = require('../../lib/page');
const {PlayersController} = require('../../lib/controllers');
Page(nativePage(PlayersController,{
  data:{name:'',avatar:'rabbit',avatarOptions:[{id:'rabbit',icon:'🐰',name:'小兔'},{id:'fox',icon:'🦊',name:'小狐'},{id:'cat',icon:'🐱',name:'小猫'},{id:'bear',icon:'🐻',name:'小熊'}]},
  nameChanged(e) { this.setData({name:e.detail.value}); },
  avatarChanged(e) { this.setData({avatar:e.currentTarget.dataset.id}); },
  select(e) { this.controller.select(e.currentTarget.dataset.id); },
  create() { return this.controller.create(this.data.name,this.data.avatar); }
}));

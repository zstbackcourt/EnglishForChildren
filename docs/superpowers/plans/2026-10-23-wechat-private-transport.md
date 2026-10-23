# 微信云托管私有通道实施计划

**Goal:** 不使用被微信拒绝的测试域名，使小程序通过官方云通道访问API和完整课程媒体。
**Architecture:** request→callContainer；asset→严格Range下载→本地文件；AudioPlayer管理异步播放生命周期；Go提供强ETag。
**Tech Stack:** 原生微信小程序JavaScript、Go标准库、Node测试、微信开发者工具。
**Spec:** docs/superpowers/specs/2026-10-23-wechat-private-transport-design.md

## 全局约束
只修改wechat；不改main；不写密钥；不访问受限管理站点；保持urlCheck:true/devLogin:false。

## 任务1：素材一致性
- [x] 在internal/game的媒体测试中先验证ETag稳定且If-Range失配返回完整200。
- [x] 修改wechat_media.go及必要gateway字段，实现内容哈希与有界元数据缓存；运行相关Go测试。

## 任务2：原生媒体生命周期
- [x] tests/miniprogram新增延迟resolve、hide、pause、切题、换角色、失败重试测试，先运行确认失败。
- [x] AudioPlayer构造器增加resolveSource=(path)=>path和isActive=()=>true；同步解析仍可播放，Promise解析受generation保护。
- [x] controllers传原始媒体路径并注入api.asset；图片独立加载，清空旧图，错误可重试；原生页面显示audioLoading/imageError。
- [x] 执行现有与新增客户端测试。

## 任务3：云传输和缓存
- [x] 测试createMediaCache(wx,send).resolve(path)对206/Range/ETag/长度/16MiB上限校验、失败清理与共享下载。
- [x] 实现lib/media.js，256KiB分段和32MiB缓存；createAPI选择cloudrun传输，保留HTTP开发路径。
- [x] 发布配置启用cloudEnv/cloudService/transport；检查脚本拒绝HTTP发布模式使用*.run.tcloudbase.com，并校验cloudrun配置；更新相应测试。

## 任务4：验收和发布
- [ ] 检查实际末块和图片ArrayBuffer，执行Go/Node/原生模板编译。
- [ ] 独立评审并修复实质问题；写变更记录并纠正域名文档。
- [ ] 更新release-id，提交推送wechat；核实main未变、云端新版健康检查。
- [ ] 官方IDE验证云端角色与课程、图片、音频；上传新开发版，记录真实结果与未完成的真机/体验版项。

实施补充：独立review发现并修复重复mkdir和删除失败漏记缓存占用；真实登录401后补充安全错误stage/code/reason。云端部署/客户端上传/体验版/真机项以实际后续回执更新。

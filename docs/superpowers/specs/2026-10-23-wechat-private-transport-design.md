# 微信云托管私有通道与本地媒体缓存

## 目标与证据
微信后台明确拒绝把 sh.run.tcloudbase.com 默认测试域名配置为服务器合法域名。官方推荐 wx.cloud.callContainer 通过微信私有链路访问关联环境，无需域名白名单。2026-10-23 官方开发者工具实测本项目 /healthz 返回 200 JSON；音频 Range 0-3、0-262143、262144-524287 返回 206、正确 ArrayBuffer 长度和 Content-Range。该验证不等于真机验收。

## 方案选择
采用 callContainer 访问 API 与分段媒体，保留现有 Tencent CloudRun 和 MySQL。另两个选择是自定义备案域名（需要用户持有域名）或迁移素材到云存储（增加存储部署配置）；本次选择不需新增云资源的私有通道。用户已授权继续开发、测试、部署且不用重复询问；仍只修改 wechat 分支。

## 接口与约束
- 配置 transport: cloudrun，cloudEnv: prod-d2g6ceg6hee15e89b，cloudService: golang-8467。apiBase 仅保留给外部部署健康检查；客户端云模式不走该 URL。不关闭 urlCheck，devLogin 为 false。
- createAPI 保持 request(path,method,data) 的鉴权、wx.login、401重试；cloudrun 模式使用 callContainer，header X-WX-SERVICE。api.asset(path) 在云模式返回 Promise<string> 本地文件路径，HTTP本地开发模式可返回字符串。
- 独立媒体模块 createMediaCache(wx,send)；send(path,{header,responseType,dataType}) 返回标准 statusCode/data/header。只允许现有素材路径白名单。
- 每块最大256KiB，严格206、Content-Range、ArrayBuffer长度、总长和强ETag；首块获取总长，后续 If-Range 保证资源一致。单文件上限16MiB，串行下载任务，同路径共享Promise。最多32MiB近期文件缓存；超限淘汰最旧文件。启动只清理自己独立缓存目录，不碰用户存档。失败删除部分文件，重试重新下载，不复用失败Promise。
- Go素材响应增加内容SHA256强ETag，复用现有路径限制与ServeContent；按已打开文件路径、size和mtime缓存哈希，避免每块重新读取整文件。哈希错误返回通用500，不暴露路径。
- AudioPlayer 注入源解析器，先停止旧播放，再解析；generation和角色有效性在异步完成前后均检查。hide/暂停/切题/提交不允许迟到音频重新启动。重试重新解析源，显示音频准备中。
- 图片独立加载，异步更新前检查课程生命周期和角色；失败不隐藏正文，可重试。预览使用已完成的本地路径。

## 验收
Node：云通道鉴权、HTTP开发兼容；Range完整性、缓存/失败重试/大小限制；音频暂停隐藏切题并发下载；图片迟到响应。Go：Range/ETag/If-Range与现有隔离测试。官方编译和开发者工具云端登录、144课、图片、完整MP3、播放与暂停。推送wechat后检查新后端版本，再上传新开发版；体验版设置和真机扫码仍须实测，不把模拟器成功写成真机通过。

官方参考：https://docs.cloudbase.net/run/develop/access/mini 、 https://docs.cloudbase.net/run/deploy/networking/custom-domains

## 实测后的补充
官方IDE中真实登录接口返回401，而本机私有凭证兑换新code成功。为区分云端配置/网络/无效code，增加受控的微信兑换错误类型与固定reason响应；只保存固定stage、数字errcode，不输出微信原始errmsg、code、openid、URL或凭证，未知错误继续泛化。此改动不降低401门禁，不替换登录身份来源。

# 将 wechat 分支部署到 golang-8467

目标服务：`golang-8467`，环境 `prod-d2g6ceg6hee15e89b`。

部署健康检查用测试域名：`https://golang-8467-333734-12-1514891213.sh.run.tcloudbase.com`。

用户已完成 GitHub 授权，并允许本地开发测试后先推送 `wechat`。这份说明对应原生小程序后端；GitHub 分支部署成功后，仍需在微信小程序后台上传客户端并设置体验版。

## 控制台部署参数

在现有服务的“部署发布”中选择：

| 配置 | 值 |
|---|---|
| 方式 | 绑定 GitHub 仓库 |
| 仓库 | `zstbackcourt/EnglishForChildren` |
| 分支 | `wechat`，不要选 main |
| 构建目录 | 项目根目录 `.` |
| Dockerfile | 根目录 `Dockerfile` |
| 端口 | `8080`（将界面默认的 80 改为 8080） |
| 健康检查 | HTTP `GET /healthz`，端口 8080 |

`wechat` 的根 Dockerfile 默认构建 `cmd/wechat-server`。原网页构建文件保留在 `deploy/ubuntu/Dockerfile.web`；`main` 的 Dockerfile 不变。不要选原网页镜像或 `cmd/server`。

## 环境变量

保留服务已有的 MySQL 连接信息，在控制台环境变量中添加或核对下列值；不写入 GitHub：

| 变量 | 配置方式 |
|---|---|
| `PORT` | `8080` |
| `WECHAT_STORAGE` | `mysql`（镜像启动时也会强制指定） |
| `WECHAT_APP_ID` | `wx1f23b102bcb34ed7` |
| `WECHAT_APP_SECRET` | 从自己的 AppSecret 文件复制纯文本内容，不能包含 RTF 格式标记 |
| `WECHAT_SESSION_KEY` | 使用 `openssl rand -hex 32` 生成，所有版本和实例保持相同值 |
| `MYSQL_ADDRESS` | 现有 MySQL 的 `主机:端口`，与服务位于可互通网络 |
| `MYSQL_USERNAME` | 现有数据库账号 |
| `MYSQL_PASSWORD` | 现有数据库密码 |
| `MYSQL_DATABASE` | 本项目建议使用 `english_adventure`；必须先在当前 MySQL 实例创建该库，再显式填写到服务的运行环境变量中 |

程序只创建独立的 `english_wechat_accounts` 表；不会清空或覆盖 Go 示例的计数器表，也不会创建云数据库资源。账号需要该库中的建表、读写权限。若数据库未开通或不可用，先在控制台处理数据库；不要改用容器本地文件存档，容器重建会丢失这些文件。

2026-10-23 的 `golang-8467-002` 日志显示 GitHub 检出和镜像构建成功，但容器首次启动因缺少 `WECHAT_APP_ID` 退出；反复出现的 `/app/cert/initenv.sh cannot exec in a stopped state` 是后续现象。新版入口会一次列出所有缺少的变量名称，绝不打印变量值。仓库外的私有 `cloudrun.env` 已准备好 AppID、AppSecret、稳定会话密钥、存储模式和端口；在云托管服务的环境变量页添加这些值，同时保留已有 MySQL 四项。若 MySQL 四项尚不存在，需先创建可用数据库并填入其连接信息。CloudRun 管理页被当前浏览器站点安全策略阻止，不能由本机代填，也不能通过上传源码替代服务环境变量。

角色、进度、奖杯、挑战题目与结算结果通过 MySQL 事务保存，同一账号并发请求会在数据库串行提交。不同账号分别保存。事务失败不会回复保存成功。签名登录会话不依赖某个容器实例。

`golang-8467-009` 已完成镜像构建，但启动日志只有 `MySQL initialization failed`，无法据此判断具体配置错误。后续镜像会输出失败阶段和安全错误分类，不输出原始驱动报错或连接值：`authentication_denied` 表示数据库账号认证被拒绝；`database_missing` 表示指定数据库不存在；`permission_denied` 表示缺少库/表权限；`connection_timeout`、`connection_refused` 或 `dns_resolution_failed` 表示网络连接问题。MySQL 返回错误时同时记录数字错误码。请根据新版本的首条启动错误核对控制台配置，避免凭泛化错误反复改值。

## 修复 011 / 012 的数据库配置失败

用户提供的 2026-10-23 日志来自两个不同版本：`011` 连接 MySQL 后收到 `1049 Unknown database`，说明它所连接的实例当时没有 `english_adventure`；游戏版本 `012` 因缺少或为空的 `MYSQL_DATABASE` 在入口检查时退出，还没有尝试连接数据库。不能用一个版本的日志证明另一个版本的环境变量已生效。

按以下顺序处理，全部完成后再重新部署：

1. 在云托管环境 `prod-d2g6ceg6hee15e89b` 所连接的**既有 MySQL 实例**中，使用数据库管理工具执行 [init-database.sql](../deploy/wechat/init-database.sql)。执行结果必须返回一行 `english_adventure`。此 SQL 仅创建逻辑数据库，不购买新实例、不删除表或数据；程序启动时再创建业务表。
2. 日志曾泄露数据库密码。通过数据库管理控制台重置受影响账号的密码，并同步所有使用该账号的服务配置，包括 `golang-8467` 的 `MYSQL_PASSWORD`。不要把新密码写到 GitHub、命令行参数或聊天中。不要把修改服务变量当成重置数据库账号本身的密码。
3. 在 `golang-8467` 的服务运行环境变量中添加独立的一行：变量名 `MYSQL_DATABASE`，变量值 `english_adventure`。保留其他必需变量，确认数据库账号对该库有建表和读写权限。填写变量不会自动建库；创建数据库不会自动填写变量。
4. 保存后重新打开配置，确认变量名存在、值非空。发布新版本时核对所采用的配置；GitHub Actions Secret、Docker 构建参数和本地 `cloudrun.env` 均不能替代运行环境变量。
5. 从 `wechat` 分支发布新版本并检查其自己的启动日志和 `/healthz`。按版本筛选日志，避免把旧模板的错误与新游戏版本混淆。只有公开就绪检查通过后，才继续上传小程序客户端。

用户随后确认已创建数据库并保存环境变量；015 游戏版本部署成功，公开及微信私有通道健康检查均通过。管理操作未由代理执行，密码轮换尚未核实。

## 构建及发布验证

Docker 构建会从两个公开仓库的固定提交拉取 144 张插图、教材页和 72 份音频，不读取本机私钥或家庭存档。首次构建需下载约 500 MiB 素材，可能较慢；构建环境必须能访问 GitHub、Go 模块源和基础镜像仓库。不能把“开始构建”当作“部署完成”。

新版本健康后验证：

```sh
curl --fail https://golang-8467-333734-12-1514891213.sh.run.tcloudbase.com/healthz
curl --fail -r 0-3 https://golang-8467-333734-12-1514891213.sh.run.tcloudbase.com/media/audio/1
```

健康响应应为 JSON；音频应为 206 部分内容，不能是访问提示 HTML。未经登录的 `/api/players` 应为 401。不要在共享日志中打印登录 token。

## GitHub 自动部署与客户端上传

推送 `wechat` 会触发已绑定的云托管 GitHub 部署，并运行 [WeChat release 工作流](../.github/workflows/wechat-release.yml)。Linux runner 拉取固定版本教材音频、运行 Go/Node 测试，并用临时 MySQL 启动生产 Docker 镜像，检查健康、登录保护和音频 Range；Intel macOS runner 用官方微信编译模块检查 WXML/WXSS。之后等待线上 API 的版本标记与本次推送匹配。每次推送前必须为 `deploy/wechat/release-id.txt` 写入新的 32 位十六进制随机值；流水线会比较推送前的值，避免旧版健康服务使客户端提前上传。镜像检查使用一次性测试密码，不访问用户数据库。云端仍需要上面的服务环境变量；工作流无法替代云托管管理权限。

仓库 Actions Secret `WECHAT_UPLOAD_KEY` 存放微信小程序**代码上传私钥**，已在用户明确授权后通过 GitHub 公钥加密接口配置；其值不在仓库或日志中。`WECHAT_APP_SECRET` 和 `WECHAT_SESSION_KEY` 只保存在云托管服务环境变量，不能放在仓库或客户端。Actions Variable `WECHAT_UPLOAD_RUNNER` 应为有固定公网出口的自托管 runner 标签 JSON，例如 `["self-hosted","linux","wechat-upload"]`；`WECHAT_UPLOAD_EGRESS_IP` 填该 runner 的公网 IPv4。先把该出口 IP 加入微信小程序的上传 IP 白名单。工作流会检查标签和实际出口地址，再调用微信上传接口。普通 GitHub 托管 runner 的出口 IP 会变化；未配置专用 runner 时工作流会明确失败，不会悄悄禁用白名单或反复尝试上传。

上传 job 只上传**开发版本**，成功时保存不含私钥的上传回执 artifact。随后仍需在微信后台将该版本设为体验版、添加体验成员、取得体验二维码，并用真机验收登录、插图、音频和存档。仅看到 GitHub workflow 绿色或云托管镜像构建成功都不能证明体验版已可用。版本回执的 `experiencePublished` 字段保持 `false`，直到有微信平台的实际体验版证据。

## 小程序访问方式：无需填写默认测试域名

微信后台已明确拒绝将 `*.run.tcloudbase.com` 配置为服务器合法域名。因此不要再把上面的测试地址填写到 request 或 downloadFile，也不要关闭发布包的域名校验。

生产配置使用微信官方 `wx.cloud.callContainer`：

| 配置 | 值 |
|---|---|
| `transport` | `cloudrun` |
| `cloudEnv` | `prod-d2g6ceg6hee15e89b` |
| `cloudService` | `golang-8467` |
| `devLogin` | `false` |
| `urlCheck` | `true`（project.config.json） |

环境必须与当前 AppID 关联。API 通过 `X-WX-SERVICE` 定位服务，继续使用真实 `wx.login` 兑换登录会话；不信任客户端可伪造的身份头。图片和MP3也走该通道，以256 KiB的Range块下载到独立缓存目录，验证强ETag、范围及完整长度后再显示/播放。每文件最多16 MiB，缓存最多32 MiB；失败分片必须清理后才能继续写入。无需配置socket、uploadFile或业务域名，本应用不使用这些接口。`apiBase` 仅供部署脚本从外部验证健康状态。

[官方私有通道说明](https://docs.cloudbase.net/run/develop/access/mini)明确该方式不需要服务器域名配置。若未来切回普通HTTP模式，才需要绑定自己的合规HTTPS域名并分别配置request/downloadFile；默认测试域名不适用。

015 已完成服务健康检查，旧客户端0.1.2026102318也曾成功上传开发版本；该旧客户端仍使用被拒绝的HTTP域名，应由本次新版本替换，不能作为可用体验版。新通道在官方模拟器中健康检查和二进制Range均通过；实际登录目前返回401，正在核对云端兑换配置。真机和体验版仍未验收。


服务器验收后，在项目根目录切换客户端配置：

```sh
cp deploy/wechat/miniprogram.config.cloudrun.json miniprogram/config.json
node scripts/sync_wechat_config.cjs
node scripts/check_wechat.mjs --release
node scripts/compile_wechat.cjs
```

发布验证时使用两个真实微信账号创建角色、完成一关、重启或发布服务新版本后重开页面，确认角色、进度、奖励与挑战重试都保留。完成后再按 [小程序上传说明](wechat-deployment.md) 上传客户端、设为体验版、添加体验者并取得体验二维码。

## 如不使用 GitHub 构建

备用源码包生成命令：

```sh
python3 scripts/package_wechat_cloudrun.py
```

生成 `tmp/wechat-cloudrun-source.zip`，只含构建所需 Go 源码、课程目录、Dockerfile 和启动脚本。此包不是微信客户端代码包，不含素材大文件或密钥。

## 回退

在服务版本管理中将流量恢复到上一个已知正常版本；保留数据库表和 `WECHAT_SESSION_KEY`。如果回到原计数器示例，小程序 API 会暂时不可用，但已存的学习数据仍在独立表中。

参考： [微信官方 Go 示例](https://github.com/WeixinCloud/wxcloudrun-golang)、[云托管无状态迁移要求](https://docs.cloudbase.net/run/best-practice/migration)、[默认域名限制](https://docs.cloudbase.net/service/alias)。


## 云端登录返回 transport 诊断

`stage=transport errcode=0` 表示请求微信code2Session时未成功得到HTTP响应，不能据此判断AppSecret错误。新版本仅在这类登录失败时返回固定的 `diagnostic: transport_<类别>`，例如 `transport_tls_unknown_authority`、`transport_dns`、`transport_context_timeout`。这些类别由Go错误类型映射，不含上游错误文本、地址或凭证；reason仍为unavailable且不签发登录令牌。

通过官方开发者工具的正常wx.login→云服务登录即可复核此字段。不要打印或上传请求code、响应token或openid；不要为了排查关闭TLS证书校验、改成HTTP或凭猜测更改云端密钥。类别确认后再针对证书信任、DNS或网络出口排查。


### TLS 未知 CA 的处理

bf2f1ec的真实云端登录确认了 `transport_tls_unknown_authority`。后续镜像为Go设置 `SSL_CERT_DIR=/etc/ssl/certs:/app/cert`，读取系统目录和平台注入的证书，保留默认系统CA bundle。此项在Docker运行环境提供，不需要重新填写微信凭证或服务域名。平台目录需允许game用户读取，并在首次TLS请求前准备完成；Go会缓存系统根证书，因此用新实例验证。该目录不可存放用户上传文件或由业务接口写入。

只接受新的真实wx.login兑换成功作为修复证据；健康检查通过不能证明TLS登录已经恢复。不要使用InsecureSkipVerify、明文HTTP或忽略证书错误来代替可信证书配置。

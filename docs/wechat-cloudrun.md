# 将 wechat 分支部署到 golang-8467

目标服务：`golang-8467`，环境 `prod-d2g6ceg6hee15e89b`。

体验测试域名：`https://golang-8467-333734-12-1514891213.sh.run.tcloudbase.com`。

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
| `MYSQL_DATABASE` | 已存在且可访问的数据库名；官方 Go 示例默认使用 `golang_demo`，以自己的环境为准 |

程序只创建独立的 `english_wechat_accounts` 表；不会清空或覆盖 Go 示例的计数器表，也不会创建云数据库资源。账号需要该库中的建表、读写权限。若数据库未开通或不可用，先在控制台处理数据库；不要改用容器本地文件存档，容器重建会丢失这些文件。

角色、进度、奖杯、挑战题目与结算结果通过 MySQL 事务保存，同一账号并发请求会在数据库串行提交。不同账号分别保存。事务失败不会回复保存成功。签名登录会话不依赖某个容器实例。

## 构建及发布验证

Docker 构建会从两个公开仓库的固定提交拉取 144 张插图、教材页和 72 份音频，不读取本机私钥或家庭存档。首次构建需下载约 500 MiB 素材，可能较慢；构建环境必须能访问 GitHub、Go 模块源和基础镜像仓库。不能把“开始构建”当作“部署完成”。

新版本健康后验证：

```sh
curl --fail https://golang-8467-333734-12-1514891213.sh.run.tcloudbase.com/healthz
curl --fail -r 0-3 https://golang-8467-333734-12-1514891213.sh.run.tcloudbase.com/media/audio/1
```

健康响应应为 JSON；音频应为 206 部分内容，不能是访问提示 HTML。未经登录的 `/api/players` 应为 401。不要在共享日志中打印登录 token。

此域名属于 CloudBase 默认测试域名。官方说明它适用于开发测试，浏览器导航会出现访问提示；其他请求也可能附带下载响应头。微信真机需要验证 API、图片和 InnerAudioContext 音频实际可用，并按后台要求配置 request/downloadFile 合法域名。正式对外运营应使用自定义域名。

服务器验收后，在项目根目录切换客户端配置：

```sh
cp deploy/wechat/miniprogram.config.cloudrun.json miniprogram/config.json
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

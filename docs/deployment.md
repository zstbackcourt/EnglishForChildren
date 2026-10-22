# 部署与分享

源码仓库：https://github.com/zstbackcourt/EnglishForChildren

## Render 一键入口

[打开 Render 部署页面](https://render.com/deploy?repo=https%3A%2F%2Fgithub.com%2Fzstbackcourt%2FEnglishForChildren)

这是部署入口，不是已经上线的游戏地址。仓库根目录的 `render.yaml` 创建一个 Go Docker 服务和 1 GB 持久化磁盘，单实例运行，关闭自动部署。使用 Starter 付费实例；费用以 Render 确认页面为准，本项目不会自动购买服务。

1. 登录 Render，打开上方入口，授权读取仓库（私有仓库需安装 Render GitHub App）。
2. 检查服务和磁盘费用后确认部署。首次构建会从固定提交下载第一册音频，需要访问 GitHub。
3. 服务 Live 后，打开 Render 分配的 HTTPS 地址。用户名 `family`，密码为服务 Environment 中自动生成的 `GAME_PASSWORD`；密码仅分享给家庭成员，不提交到 Git。
4. 创建角色，播放第一课并通关；重新部署后检查角色和奖杯仍在。完成这一步后再分享游戏地址。

`PORT` 由平台提供，程序监听 `0.0.0.0:$PORT`。`/api/health` 不要求密码，其他页面、音频和存档 API 均受家庭密码保护。所有角色仍属于一个共享家庭空间；该密码不是每个孩子独立的账号。

Render 免费服务的临时文件系统不能保证存档，必须保留 `/app/data` 的磁盘挂载。参见 [持久化磁盘](https://render.com/docs/disks)、[Blueprint 配置](https://render.com/docs/blueprint-spec)、[部署入口](https://render.com/docs/deploy-to-render)。

## 自有服务器 Docker

安装 Docker Engine 和 Compose 后：

```bash
git clone https://github.com/zstbackcourt/EnglishForChildren.git
cd EnglishForChildren
# 在 .env 写入 GAME_PASSWORD=自行选择的家庭密码；该文件已被 Git 忽略。
docker compose up -d --build
```

默认绑定服务器的 `127.0.0.1:8080`，通过已有 HTTPS 反向代理绑定域名后分享。家庭密码使用浏览器原生登录框，公网必须经 HTTPS。局域网使用可将 Compose 端口改为 `8080:8080`。数据保存在 `game-data` 命名卷；停止服务使用 `docker compose down`，不要加 `-v`，后者会删除存档卷。

若宿主机已有 8080 服务，修改端口映射左侧，例如 `127.0.0.1:8090:8080`。容器入口在运行时初始化挂载目录所有权，随后降权为 `game` 用户运行服务；如自行覆盖容器用户，则需自行保证挂载目录可写。

## 无 Docker 运行

安装 Git、Python 3、Go 1.22+，克隆本项目后执行：

```bash
bash scripts/fetch_assets.sh
./start.sh
```

脚本下载固定版本的第一册音频，保留现有素材仓库，不覆盖修改。默认本机地址 `http://127.0.0.1:8080`。需要云端运行时设置 `PORT`、`GAME_PASSWORD`，并将存档路径放到持久化磁盘：

```bash
./start.sh -save /持久化目录/save.json
```

源码中的 `web/assets/pages` 与课程 JSON 已生成，普通部署无需 OCR 或下载完整 PDF。教材原仓库不重复提交，音频在构建/安装时获取；版本固定为 `635746ac5f696b193311bc1a7e8ffac3b252eb75`。

## 运维与备份

### Ubuntu 原生服务部署

已有 Ubuntu ECS 可使用 `deploy/ubuntu` 的 systemd 配置，无需 Docker。将 Linux amd64 二进制命名为 `english-adventure`，与 `web`、`content`、第一册音频目录一起放在 `/opt/english-adventure`。Caddy 官方 Linux amd64 二进制也放在该目录；部署前核对官方摘要。当前准备使用 Caddy 2.11.7。

- 游戏：`english-adventure.service`，仅监听 `127.0.0.1:8080`。
- 存档：`/var/lib/english-adventure/save.json`，由 systemd 的 `StateDirectory` 管理权限；更新程序时保留此目录。
- 家庭密码：`/etc/english-adventure/environment`，内容为 `GAME_PASSWORD=随机长密码`，文件权限 `0600`、所有者 root。公网启动前必须配置；用户名为 `family`。
- HTTPS：`english-adventure-proxy.service`。将 `Caddyfile.example` 的两个示例 IP 替换为真实公网 IP，保存为 `/etc/english-adventure/Caddyfile`。
- 证书与 ACME 账户：`/var/lib/english-adventure-proxy`，不要在更新时清除；Caddy 自动续期。
- 安全组仅需增加 TCP 80 和 443 入站；8080 不对公网开放。80 用于证书验证和跳转 HTTPS。服务运行在受限动态用户下。

先复制服务文件至 `/etc/systemd/system`，执行 `systemctl daemon-reload`，启动游戏并检查 `curl -f http://127.0.0.1:8080/api/health`。配置家庭密码、确认公网规则和证书协议后，再执行 `systemctl enable --now english-adventure-proxy`；密码变更后需 `systemctl restart english-adventure`。

没有域名时可尝试申请公网 IP 短期证书；必须验证实际签发和外网连通性后才交付链接。参考 [Let's Encrypt IP 证书](https://letsencrypt.org/2026/01/15/6day-and-ip-general-availability)、[证书协议](https://letsencrypt.org/repository/)、[Caddy ACME profile](https://caddyserver.com/docs/caddyfile/directives/tls)。这不是网站备案要求的替代说明。

常用检查：`systemctl status english-adventure english-adventure-proxy`；代理日志：`journalctl -u english-adventure-proxy -n 60 --no-pager`。程序更新前备份存档，停止游戏服务、替换程序与资源，再启动并复验；不要让两个进程同时写同一存档。

- 单实例、单进程写一个存档；不配置多副本负载均衡。
- “家长指南 → 下载家庭存档备份”导出全部家庭角色。恢复前停止服务并保留旧文件副本。
- 升级代码前下载备份；更新后至少验证播放、一次通关、刷新、重启恢复。
- 修改家庭密码后浏览器可能缓存旧密码，可关闭浏览器或在新隐私窗口重新登录。
- 教材及音频来源见素材审计，源素材权利不因游戏源码提交而改变。

## 当前验证范围

已验证 macOS 本地运行、全部自动测试与 Linux amd64 二进制交叉编译。当前工作机未安装 Docker，未执行容器构建或真实 Render 部署；一键配置不能替代部署完成后的浏览器验收。

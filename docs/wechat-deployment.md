# 微信小程序部署与体验版验收

AppID：`wx1f23b102bcb34ed7`。本分支采用原生页面和独立 Go API；现有网页、网页家庭密码、网页存档不迁移、不覆盖。微信账号拥有独立家庭空间，可在里面创建多个角色。

## 当前交付边界

代码和本地自动测试不等于微信体验版发布。用户已提供 AppID、CloudBase 测试域名及本机私有凭证；实际服务器配置、合法域名、真机登录与平台上传仍需验证。当前选定目标是微信云托管，优先阅读 [golang-8467 分支部署说明](wechat-cloudrun.md)。下文 Ubuntu 为备用部署方式。

微信后台 URL 中的 token 不是 AppID 或上传凭证。不要提交 token、AppSecret、上传私钥和存档；AppID 可公开。浏览器安全策略目前阻止自动访问微信后台，未绕过。

## 1. 本地运行

要求 Go 与 Node；上传工具及其锁定依赖要求 Node >= 20.19.0。以下在 `wechat` 工作区执行：

```sh
bash scripts/fetch_assets.sh
go run ./cmd/wechat-server -root . -addr 127.0.0.1:8093 -save-dir data/wechat -dev-login
```

另一个终端执行：

```sh
go test -race ./...
node --test tests/*.test.mjs tests/miniprogram/*.test.cjs
node scripts/check_wechat.mjs
npm ci --prefix tools/wechat-ci
node scripts/compile_wechat.cjs
```

需要不含其他源码和工具的独立导入目录时，运行 `node scripts/prepare_wechat.mjs`。脚本会输出 `tmp/wechat-import-时间戳/`，开发者工具可直接导入该目录。服务器验收后，可用 `node scripts/prepare_wechat.mjs --cloudrun --release` 生成带体验域名、关闭开发登录的导入目录；此命令不上传、不需要私钥，不覆盖已有目录。

本地 `config.json` 使用 `http://127.0.0.1:8093` 和 `devLogin: true`。该模式只接受回环请求及固定开发身份，不能对公网部署。开发者工具导入项目根目录，读取 `project.config.json`；本地调试时可在工具的本地设置中临时关闭域名校验。该私有设置不提交；发布必须启用校验并关闭开发登录。手机的 `127.0.0.1` 指手机自身，不能据此进行真机验收。

## 2. Ubuntu 独立部署

将本分支代码和课程素材复制到全新目录 `/opt/english-adventure-wechat`。不要 checkout、重置或覆盖 `/opt/english-adventure`。构建：

```sh
cd /opt/english-adventure-wechat
bash scripts/fetch_assets.sh
mkdir -p bin
go build -trimpath -o bin/wechat-server ./cmd/wechat-server
```

服务器创建专用用户（若已存在则跳过创建）、配置目录和私有环境文件：

```sh
sudo useradd --system --no-create-home --shell /usr/sbin/nologin english-wechat
sudo install -d -m 700 /etc/english-adventure-wechat
sudo install -m 600 deploy/wechat/server.env.example /etc/english-adventure-wechat/server.env
sudoedit /etc/english-adventure-wechat/server.env
```

替换所有示例值。`WECHAT_SESSION_KEY` 用 `openssl rand -hex 32` 在服务器生成，保持稳定；轮换会使旧登录失效，存档保留。不得在命令、日志或截图中暴露 AppSecret。确认应用文件可被专用用户读取：

```sh
sudo -u english-wechat test -r /opt/english-adventure-wechat/content/book1.json
sudo install -m 644 deploy/wechat/english-adventure-wechat.service /etc/systemd/system/
sudo systemctl daemon-reload
sudo systemctl enable --now english-adventure-wechat
curl --fail http://127.0.0.1:8093/healthz
sudo systemctl status english-adventure-wechat --no-pager
```

生产命令没有 `-dev-login`；存档只写 `/var/lib/english-adventure-wechat`。定期备份该目录和服务器私有环境文件到受限位置。不要把它们放进网站静态目录或 Git。

将 `deploy/wechat/Caddyfile.example` 中的域名替换为自己的域名，作为独立站点块追加到当前 Caddy 配置，验证后 reload 原代理服务。不要覆盖当前 IP 网站配置。阿里云 DNS 的 A 记录需指向 `47.111.138.33`；只复用现有 80/443，8093 继续只监听本机。

微信后台为该域名配置 request、downloadFile 等所需合法域名。素材服务无网页 BasicAuth，图片/音频可公开读取；角色、答题和存档始终需要 Bearer 登录。修改 `miniprogram/config.json` 为实际 `https://你的域名`，`devLogin: false`。用微信开发者工具保持域名检查开启测试，确认当前账号实际后台要求。

## 3. 真机验收

在开发者工具导入、编译并生成预览，逐项记录设备、微信版本和结果：

- 两个真实微信账号各自创建角色；彼此看不到对方的角色、进度和奖励。
- 144 课地图和不同章节插图可加载；点击图片可看完整图。
- 奇数课原声音频逐句停止、连续字幕、跳句正常；偶数课明确提示配对音频。
- 进入挑战、切页和返回后台时旧音频停止；断网和音频加载失败后可重试。
- 三档挑战，选择、排序和填空题可完成；服务端判分，重复提交不重复发星光。
- 退出微信再打开、切角色、服务器重启后进度仍正确。
- 分享路径不包含角色名、token 或存档 ID；获得分享的体验者必须具备体验资格。

真实 wx.login、微信编译和手机播放属于平台验收，Node 模拟 wx 边界的测试不能替代。

## 4. 上传开发版本并设为体验版

准备上传密钥时保存在仓库之外的私有文件，例如 `~/.config/english-adventure/private.key`，权限设为 600。不要发到聊天或复制进项目。后台配置允许的上传 IP。

```sh
npm ci --prefix tools/wechat-ci
node scripts/check_wechat.mjs --release
export WECHAT_APP_ID=wx1f23b102bcb34ed7
export WECHAT_UPLOAD_KEY_PATH="$HOME/.config/english-adventure/private.key"
export WECHAT_VERSION=0.1.0
node scripts/upload_wechat.cjs --upload
```

脚本使用官方 `miniprogram-ci@2.1.31`，只暂存白名单内的小程序代码和项目配置，拒绝测试文件、环境文件、目录符号链接、存档和密钥。成功后在本机 `tmp/wechat-upload-版本-时间戳.json` 记录本次上传；失败不会新增成功记录，历史凭据按版本和时间保留。开发版本上传成功后，仍须在微信后台版本管理设为体验版、添加体验成员并取得体验二维码，不能把预览二维码当作体验版二维码。

用户可改用已登录的微信开发者工具“上传”。两种方式都必须记录实际成功证据。按用户最新授权，本地开发测试完成后先推送 `wechat` 供云托管构建：

```sh
git status --short --branch
git push origin wechat
```

代码已推送不能代表体验版已发布。`main` 保持 `7a4c9a63cca2471ec8b7157e0b36cf2bc1cacbef`。

## 回退

停止 `english-adventure-wechat` 并仅移除新增域名的代理配置，可停用小程序 API；保留 `/var/lib/english-adventure-wechat` 以便恢复。现有网页不依赖此服务。回退小程序版本在微信后台操作，不删除用户数据。

工具接口核对来源：官方发布的 [miniprogram-ci npm 包](https://www.npmjs.com/package/miniprogram-ci) 2.1.31 内 README 与类型声明；原生页面参考 [微信官方示例](https://github.com/wechat-miniprogram/miniprogram-demo)。当前微信后台可选项以实际账号界面为准。

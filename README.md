# 英语小小冒险家

新概念英语第一册的家庭学习网页游戏。Go 后端，原生 JavaScript 前端，无数据库和 npm 安装步骤。角色可自定义名字、选择头像，进度与奖励保存在本机。

## 启动

首次从 GitHub 克隆后，先执行 `bash scripts/fetch_assets.sh` 下载第一册音频（需要 Git、Python 3 和网络）。已下载完整素材仓库的当前工作区无需重复下载。

在本目录执行：

```bash
./start.sh
```

打开 <http://127.0.0.1:8080>。终端按 Ctrl+C 停止。要求 Go 1.22 或更新版本；本次已在 `.tools/go` 安装项目专用 Go 工具链，脚本会自动使用。默认只监听本机。

换端口：

```bash
./start.sh -addr 127.0.0.1:8090
```

前端文件改动后刷新即可；Go 代码改动后重启 `start.sh`。如需在家庭局域网另一台设备访问，可自行以 `-addr 0.0.0.0:8080` 启动并访问本机局域网 IP。此模式没有账号密码，仅适合可信家庭网络。

## 已实现

- 第一册 144 个单元，12 个章节，72 组原仓库美音 MP3/LRC。
- 名字角色、四种头像、多角色独立存档与切换。
- RPG 式对话、字幕定位、单句重播、连续播放、0.75×/1×/1.25×。
- 三档挑战：听句选项、补词/单词排序、单词听写。
- Go 服务端固定题目、判分、去重领奖，铜银金 432 枚可收藏奖杯。
- 保存每课对话位置、最后难度、各档最高分/次数/奖杯；刷新或重启继续。
- 对应课次教材页、首课彩色车厢场景、桌面与手机响应式页面。
- 家长指南和存档备份下载。

每档每课首次通关获得 10/20/30 星光，通关门槛依次是 3/5、4/5、5/5。重复挑战提升成绩，不重复发放星光。

## 内容边界

这是首个可玩的全册框架版本，内容精修仍可继续：

- 奇数课是原声故事，偶数课展示本课教材插画并复用前一课音频做巩固；仓库没有 72 份独立偶数课录音。
- 偶数课当前是游戏派生题，不是教材全部书面题逐题复刻。
- 第一课完成彩色背景重绘、中文释义和角色标注；其他课使用对应教材扫描页的纸艺舞台与原英文字幕。未完成逐课彩色重绘和全册人工翻译。
- 题目根据台词生成；原 LRC 尚未全部人工听校。短对话若可用独立句子不足，题目可能复用句子。
- 当前保存学习位置与已提交成绩；未提交的测验仅保留在页面/服务内存中，关闭页面或重启服务后需要重新开始挑战。
- 家庭角色无密码；不同家庭成员可在同设备选择角色。不是公网多用户账号系统。

## 存档与备份

正式存档：`data/save.json`。页面“家长指南 → 下载家庭存档备份”可导出 JSON。恢复时先关闭服务，保留当前文件副本，将备份复制为 `data/save.json`，再启动。存档格式版本为 1；非法或损坏文件会阻止启动，不会自动清空。

测试存档位于 `tmp/qa-save.json`，与正式存档分开。浏览器只存当前角色 ID，清理浏览器数据不会删除服务器进度。

不要同时运行两个进程写入同一个存档文件；需要多实例时为每个进程传不同 `-save` 路径。默认的家庭模式只有一个服务进程。

## 项目结构

```text
cmd/server/       Go 入口
internal/game/    内容模型、原子存档、题目生成、HTTP API、测试
web/              前端与导出的教材页、美术素材
content/          第一册课程和时间轴目录
scripts/          教材 OCR、LRC 导入与完整性测试
New-Concept-English/ 原始教材仓库（独立 Git 仓库，不修改）
data/             家庭存档（不提交 Git）
docs/             方案、实施计划、变更、美术记录、素材与验收报告
```

## 验证

```bash
GOCACHE=/tmp/english-go-cache .tools/go/bin/go test -race ./...
GOCACHE=/tmp/english-go-cache .tools/go/bin/go vet ./...
node --test web/*.test.js
node --check web/app.js
python3 scripts/test_content.py
```

已生成的 `content/book1.json` 和教材图片可直接使用，无需每次启动 OCR。重新导入字幕：`python3 scripts/import_content.py`。重新生成页面及课号识别（仅 macOS，需 Swift、PDFKit 和 Vision）：

```bash
swift -module-cache-path /tmp/english-swift-cache scripts/ocr_pdf.swift \
  'New-Concept-English/新概念英语1 英语初阶.pdf' web/assets/pages
python3 scripts/import_content.py
```

来源仓库保持原始素材；导入器检测到缺失音频或课号时中断。逐课页码和已知内容限制见 [素材审计](docs/content-audit.md)。

## 文档

[一键部署到 Render](https://render.com/deploy?repo=https%3A%2F%2Fgithub.com%2Fzstbackcourt%2FEnglishForChildren)：先查看服务和磁盘费用，再确认部署；这不是已上线的游戏链接。支持 Docker 持久化部署，具体步骤见下方说明。

- [设计方案](docs/design.md)
- [实施计划](docs/implementation-plan.md)
- [每次改动与优化](docs/changes.md)
- [逐课素材审计](docs/content-audit.md)
- [美术来源和生成提示词](docs/art-direction.md)
- [验证报告](docs/verification.md)
- [部署与分享](docs/deployment.md)

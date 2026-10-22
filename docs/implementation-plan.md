# 英语小小冒险家实施计划

**目标：**交付可本地运行的第一册家庭英语学习游戏。

**架构：**Go 标准库提供课程、音频、角色及测验 API，JSON 原子存盘；前端 ES modules 实现地图、对话、题目和奖励；Python 导入 LRC，扫描 PDF 经本地 OCR 建立课号映射。

**方案：**[设计文档](design.md)。采用当前新工作区开发，原始教材仓库保持独立。用户已要求先文档再开发，按此顺序直接执行，不另设设计批准门槛。

## 项目约束

- 144 个独立课次；72 组原美音；偶数课明确复用配对课录音。
- 服务器保存多角色、对话位置、逐课逐难度进度和奖杯。
- 默认家庭可信设备模式；后续按用户补充要求增加可选 Docker/Render 部署和家庭访问密码，无第三方应用运行时依赖。
- 每个任务完成或发现问题时更新 changes.md。

## 任务 1 素材和导入

- [x] 写 `scripts/test_content.py`，断言 144 课唯一连续、音频 72 组存在、时间递增、课次页码可靠、正文不含录音指令。
- [x] 执行测试确认内容文件缺失时失败。
- [x] 创建 `scripts/import_content.py` 与 `scripts/ocr_pdf.swift`；产生 `content/book1.json`、课程页图和 OCR 缓存。
- [x] 抽查首课、偶数练习课、中段与末课；输出 `docs/content-audit.md`；运行素材测试。

## 任务 2 后端

- [x] 创建 go.mod 与 `internal/game/game_test.go`，测试建角、角色隔离、重启、奖杯幂等、各难度门槛、无效输入。
- [x] 运行 `go test ./...` 确认缺失实现的失败。
- [x] 实现 `internal/game/models.go`、`store.go`、`quiz.go`、`http.go` 与 `cmd/server/main.go`。
- [x] 运行 `go test -race ./...` 和 `go vet ./...`。

接口分工：LoadCatalog(path) 返回目录；OpenStore(path) 返回存档；NewHandler(root, catalog, store) 返回 http.Handler；题目生成与答案判分独立于 HTTP。服务器用 crypto/rand 建角色和测验 ID，写入成功后才更新内存状态。

## 任务 3 前端闭环

- [x] 创建 `web/index.html`、`web/styles.css`、`web/app.js`、`web/audio.js`、`web/api.js`。
- [x] 完成新建/切换角色、地图章节、教材页展示、逐句播放/速度、自动进度保存。
- [x] 完成三难度、选择/排序/听写、服务端提交、反馈和奖励室。
- [x] 加入离线错误反馈、加载/禁用状态、键盘操作、手机响应布局与减少动画偏好。

## 任务 4 美术

- [x] 从已核实教材页创建第一课彩色场景，保存提示词、来源与成品。
- [x] 为所有课接入对应教材页和统一舞台外观，记录哪些为重绘版本。
- [x] 检查图片比例、对话可读性及手机舞台。

## 任务 5 验收

- [x] 本地启动，验证 API、MP3 Range、持久化和错误请求。
- [x] 浏览器完成建角→选课→播放→测验→领奖→刷新恢复。
- [x] 运行后端测试、素材审计、前端语法检查。
- [x] 更新 README、changes、content-audit，列出已验证与未验证项目。

## 任务 6 部署与交付（用户追加）

- [x] Docker 多阶段构建、固定源素材版本、持久化卷和家庭密码。
- [x] Render 一键部署入口配置和部署说明。
- [x] Go race、前端回归、内容完整性和 Linux 交叉编译。
- [x] 推送至用户指定远程仓库；首次版本 02d9aef 已推送 main。
- [x] 用户选择先提供一键部署入口，暂不购买云服务；实际云部署留待后续明确要求。

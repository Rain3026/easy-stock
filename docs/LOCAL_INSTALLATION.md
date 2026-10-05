# EasyStock 初始本地安装与验收记录

核验日期：2026-10-04，Asia/Shanghai。安装位置：`/Volumes/MAC MINI EXT/EasyStock`。

初始安装源码来自 https://github.com/jundizhou/easy-stock ，版本 v1.3.1，提交 `11c82513d550652493a16ee470fda896c9c79eff`。实际完整提交号以 `.runtime/evidence/installation-result.json` 为准。以下版本和验收数据是初始安装时的记录；当前源码版本以 Git 为准。保留上游源码、历史、许可证及版权声明。

本地个性化开发使用分支 `oss-main`，跟踪用户 fork 的 `fork/main`。`fork` 对应 https://github.com/Rain3026/easy-stock ，`upstream` 对应原作者仓库。用户要求同步时使用 `git push fork oss-main:main`；不提交 `.runtime/`、个人设置、数据库、登录凭据或构建产物。

## 启动与停止

- 桌面版：双击项目根目录的 `启动EasyStock.command`。独立程序位于 `desktop/dist/builder-dir/mac-arm64/easy-stock.app`，也可直接双击该 `.app`。
- Web 版：双击 `启动Web.command`，访问 http://127.0.0.1:20073 。后端为 http://127.0.0.1:20081 。
- 停止 Web：双击 `停止Web.command`。桌面版从应用菜单退出。

Web 服务通过 macOS 自带 `screen` 在后台运行，重复启动会复用本项目进程。遇到其他进程占用端口时会报错，不会终止其他服务。

Web 数据保存于 `.runtime/web-data/`，服务日志为 `.runtime/backend.log` 和 `.runtime/frontend.log`。桌面数据保存于 `~/Library/Application Support/easy-stock/`，其中 `logs/` 保存桌面、后端和渲染日志。两套数据各自独立；模型设置和平台登录也需分别配置。

## 已安装依赖

| 组件 | 实际版本 |
| --- | --- |
| Go | 1.26.8 |
| Node.js / npm | 22.23.3 / 10.9.9 |
| Python | 3.11.15 |
| Electron | 38.8.6 |
| Hermes Agent | 0.21.3 |
| Codex CLI | 0.159.0 |
| agent-browser | 0.25.3 |
| 微信采集服务 | `043c2f9828401220a00b7b125686b334581745e0` |

Go 和 Node 安装于 `.runtime/tools/`，下载包通过官方 SHA256 校验，不改变其他项目使用的工具版本。Hermes、Codex、Python、微信服务及浏览器辅助二进制已打入 `.app`，桌面启动不依赖全局 Go、Node 或 Python。

从终端运行源码命令时，先执行：

```bash
cd '/Volumes/MAC MINI EXT/EasyStock'
source scripts/local-env.sh
npm test
npm --workspace desktop test
```

## 测试与构建证据

| 检查 | 结果 | 日志（位于 `.runtime/evidence/`） |
| --- | --- | --- |
| npm 工作区依赖安装 | `npm ci` 成功，保留原锁文件 | `npm-ci.log` |
| 后端全量测试 | 22 个含测试的 Go 包通过 | `core-tests-final.log` |
| 前端测试 | 25 个文件、160 项通过 | `core-tests-final.log` |
| 桌面测试 | 63 项通过、1 项 Windows 专属测试跳过 | `desktop-tests-final.log` |
| 原生 Codex 与 MCP 集成 | 59 项通过（包含子用例），使用本地测试模型接口 | `native-runtime-tests.log` |
| Hermes Python 参数与协议 | 17 项通过 | `python-runtime-tests.log` |
| 真实行情接口 | 9 项通过 | `live-tests.log` |
| 完整桌面资源准备 | 成功 | `prepare-package-final.log` |
| TypeScript 与 Vite 构建 | 成功 | `frontend-build-final.log` |
| macOS arm64 `.app` 构建 | 成功 | `mac-package-final.log` |
| 包完整性、独立 Python、Codex 握手、签名 | 成功 | `mac-package-verification-final.log` |
| 打包前端与已验收构建一致 | 全部文件 SHA256 相同 | `installation-result.json` |
| Web 页面与布局 | 八个主要入口可打开，题材切换、股票查询可用，1280/980 无页面或指标横向溢出 | 页面截图、DOM 记录、`layout-checks.json` |

已实际启动桌面程序，确认本地后端、内置微信服务、行情页面及模型设置入口正常。最终样式修复后的 `.app` 已重建并通过签名与资源校验；最终桌面重启检查因 Mac 锁屏暂未完成。Web 已查看同一最终前端构建的实际渲染。

真实接口验证覆盖多市场报价、K 线、财联社新闻及开盘啦题材/涨停池。东方财富 K 线在当前网络返回 EOF，其健康信号测试通过，统一行情接口已通过新浪回退验证。

## 本地修复

1. 修复发布脚本在 macOS Bash 3.2 的 `set -u` 下展开空数组失败；原有上传重试测试通过，并增加失败诊断信息。
2. 修复原生 MCP 集成测试的 SSE 服务清理卡住；退出时先关闭活动连接，保留全部协议与凭据隔离断言。
3. 修复默认桌面窗口下题材指标被挤成竖排；指标列按可用宽度换行，辅助信息独占一行，实际渲染已核验。
4. 添加使用项目内工具链和独立 Web 数据目录的启动/停止入口。

初始安装验收结束时，上述改动保留在工作目录，尚未提交或推送。

## 需要个人配置的功能

在「系统设置 → 大模型和运行时配置」填写模型服务、模型名称和 API Key，再执行连接测试。Hermes 与 Codex 的程序及本地协议已验证，真实付费模型调用尚未验收。

雪球、淘股吧及微信内容采集需在桌面设置中完成对应账号登录或扫码。Tushare 需个人 Token。微信登录仅用于具体文章链接解析，上游当前不提供历史文章列表自动订阅。

这些个人账号功能不能记作已完成真实业务验收。本次验证使用新建的 EasyStock 数据目录，没有复用其他 Hermes/Codex 项目的凭据。

`npm audit` 对上游锁定依赖报告 22 项告警（18 high、3 moderate、1 low），完整结果见 `npm-audit.json`。本次复建保留上游版本，未执行可能升级主版本的 `npm audit fix --force`。当前构建只验收 macOS Apple Silicon；Windows 和 Intel macOS 未在本机运行验证。

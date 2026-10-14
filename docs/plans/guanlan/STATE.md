# 观澜与看板组件：接续状态

更新时间：2026-10-15，Asia/Shanghai。

## 当前结果

用户明确“开工”，并要求先组件库、我的看板/编辑器，再观澜。已按此顺序实施；30项全部完成，账本状态complete、next_task为空。功能已接回原本地网站，最终Web与macOS增量验收均通过。

- 分支oss-main，开发验收基线HEAD a4dcf5869b700dd87912c3566f6cf2903efbff05。目标fork=https://github.com/Rain3026/easy-stock.git；upstream仍只作上游。开发验收期间没有提交推送或全局安装替换；现已获用户明确授权，将必要依赖和观澜分开提交推送，范围及隔离检查见[COMMIT-SCOPE.md](COMMIT-SCOPE.md)。最终提交以当前Git为准。
- 本地网站：<http://127.0.0.1:20073/#boards/library>，后端20081。原今日总览/默认首页及五工作区保持；底部“看板与组件”在系统设置上方。
- 组件库八类、默认/预设/引用管理；我的看板配置历史每页两条；右侧新建面板、左画布右组件库、拖入/+、八方向缩放、数据/尺寸/样式、草稿恢复和冲突保护均落地。
- 一个观澜页面只展示启用版；保存、预览、选中和启用分开。历史配置不可变，别名/归档为独立元数据。首次未配置显示引导；日常环境未导入任何验收配置、预设和名单。
- 复用既有行情/规则缓存及已有Agent结果，布局动作不触发AI。Skill/MCP安装、模型运行时与凭据保留在原系统设置。

## 验证与保护

前端245项测试/41文件、最终类型检查与构建通过。七项Store/四项HTTP专门测试race通过，相关Go三个包完整回归通过。桌面64项通过、1项原有跳过。Web真实操作、原工作区入口、1120/390px、控制台与设计对照已完成；实际截图及逐项证据见[evidence/delivery-verification.md](evidence/delivery-verification.md)。

macOS隔离应用已实际完成添加、鼠标/键盘移动、数值尺寸、鼠标缩放440×276、保存启用及重启保留。用户解锁后完成最终Pointer Capture空画布拖入0→1、保存启用V6、退出重启保留V6、新建右侧面板和组件类型布局示意的实际复验。发现默认/预设长表单挤出底部按钮，已修复为内容独立滚动、操作固定，macOS和Web390px复验通过；最终类型检查、生产构建及10项针对回归通过。原锁屏阻塞已解除。

开工备份保存454个既有路径、设置、六个SQLite一致性副本及浏览器方案/草稿，受保护路径见`.runtime/guanlan/backup-path`。原服务更新前六库完全一致；更新后设置、持仓、浏览器方案/草稿、研究事实/任务和历史研究保持，模型用量未变。题材行情缓存正常刷新，变化及既有保留规则已单独核对，见final-protection.json和market-cache-refresh.json。没有覆盖冻结判断。

原20073的Vite preview保持；20081更新为当前构建，在独立screen会话easy-stock-guanlan-web运行。启动命令`.runtime/guanlan/run-web.sh`；screen必须设置TERM=xterm-256color。重启/退出前以当前lsof/ps核实归属和在途任务，不使用本文件的旧PID推断。

## 收尾状态

- GL-028最终截图、几何及数据核对见evidence/GL-028-final-native.json和GL-028-final-native-protection.json；任务清单全部勾选。
- 本次隔离桌面实例已正常退出；临时浏览器验证页关闭，视口恢复；原20073/20081服务保留，用户可直接使用新功能。
- 使用入口为左下“看板与组件”→“我的看板”→“新建配置”；编辑后“保存并启用”，进入观澜查看。测试V6等配置仅留在隔离desktop-data，日常guanlan库仍为空。
- 最终只读比对确认原settings、daily-research、portfolio-inspections、reviews和stock-research内容及模型用量保持。行情缓存按既有规则正常更新，不能将缓存变化解释为业务数据改写。
- Windows/Linux未实际运行、全局安装未替换、安装包未发布；这些边界不变。本任务没有新增开发待办，也未自动提交推送。

## 文件与边界

- 使用：[USAGE.md](USAGE.md)；合同：[CONTRACT.md](CONTRACT.md)；状态以tasks.json/TASKS.md为准。
- 最终设计权威为frontend/design/guanlan-config-versions-v1说明及三图，保留旧稿中未冲突的使用页、库、指数、尺寸/样式。设计图片未改写。
- 隔离Web20075/后端20083的配置、研究副本及日志只用于验收；临时服务及最后一次隔离Electron实例均已退出，原20073/20081保留。临时布局原型HTML/TSX和测试标签页已移除，必要截图、JSON及受保护数据副本保留。
- 原ES5-004/041及easystock-es5行情验收不由本任务接管，记录和调度未改。上一交易日缓存/已有报告未被计作新盘中样本，未宣称量能预测已验收。
- 任务账本不是Codex系统Goal，不创建新调度；无自动提交推送。必要恢复只选择具体受影响对象，不能覆盖后来新增数据。

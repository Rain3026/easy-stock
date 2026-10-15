# 观澜实施合同（GL-002）

本合同落实用户开工及顺序补充：先组件库/我的看板/编辑配置，观澜最后接通。

- 新后端包`backend/internal/guanlan`，配置存储`guanlan.db`位于现有settings.json同级；测试用临时目录或内存。无settings路径时使用内存；不迁移旧业务表。
- 新管理路由`#boards/versions`、`#boards/library`、`#boards/edit?draft=ID`；使用页`#guanlan/overview`延后接通。原五工作区与默认首页保留。
- 配置schema=1；配置含名称、实例数组、布局偏好。实例含唯一ID、类型/schema、受支持参数、网格位置/宽高与样式。存储不含金融结果或凭据。
- 草稿可变且带revision；保存新增不可变版本。版本元数据别名/归档与配置快照分离；版本号不复用。启用指针有独立revision并在事务中切换。
- 保存幂等以草稿ID+revision定位同一保存；修改草稿后保存才产生新版本。双窗口写入必须提供expected_revision；冲突保留本地编辑，不能静默覆盖。
- 新建/复制先生成草稿；复制可分开复制数据参数与尺寸样式，实例ID重新生成。保存不自动启用，保存并启用需同时校验启用revision；失败事务回滚。
- 独立库和右侧库以同一后端注册描述为准。系统默认→用户默认/预设→实例快照，后续默认/预设改动不追改实例；隐藏不删除历史实例。
- 历史列表默认2条/页、时间倒序；选中、预览、保存、启用分开。归档保留恢复；启用版本禁止归档。
- 缩略图依据布局与组件名称绘制示意，不采集真实持仓和行情；配置旧不代表行情旧。
- 布局采用高分辨率网格与成熟React布局库；组件支持8方向缩放和数值输入。以120等分横坐标和12px纵向单位保存有限小数，渲染/拖动按像素换算；关闭吸附可逐像素调整，开启吸附按20×12px对齐。旧整数配置无需迁移，窄屏只重排显示，不覆盖桌面布局。

## 八类数据合同

| 类型ID | 数据入口 | 主要参数 | 状态及边界 |
| --- | --- | --- | --- |
| volume | GET /api/v1/daily-research/overview.pulse | 历史显示天数、均线/预测显示、实例参考线 | source/date/at/fetched_at/stale/gaps；不更改原观察线 |
| indexes | 同overview.indexes | index_ids、mode、fields | 支持源已有12指数；逐项源时间/缓存；NDX与IXIC区分 |
| breadth | 同overview.pulse.breadth | fields | 同口径数据复用，不填假零 |
| emotion | 同overview.pulse.emotion | rows | 涨跌停、最高连板、梯队；partial明确 |
| themes | 同overview.themes/theme_meta | rows、sort | 主线交易日及stale；详情返回机会发现 |
| debate | 已保存dailyresearch records/debate及其snapshot | snapshot_id、result_id、subject | 只读已保存结果；响应带created_at、证据cutoff、模型/Skill引用；无结果不分析 |
| risk | GET /api/v1/portfolio-inspections（按plan过滤）及详情 | plan_id、result_id | 只读已有报告/任务状态，不启动巡检 |
| watchlist | 新最小共享关注名单+原GET /api/v1/quotes/realtime | list_id、fields、sort | 当前未发现已有独立名单服务；不复用成真实持仓，不删业务事实 |

现有今日总览固定组合字段可复用，但不修改原组件；新结果适配器按能力共享读请求，允许缓存缺失时走原服务有界补取。研究结果与行情独立加载，不以慢AI阻塞其他卡片。市场判断仍是确定性规则，不伪装AI。

API前缀`/api/v1/boards`：catalog、state、drafts、versions、active、library、presets、watchlists。遵守现有令牌/同源处理。请求体限制、schema/类型/参数/范围校验、revision冲突409、未找到404；无任意脚本/MCP执行接口。

已有ES5-004/041真实数据验收仍按原计划；本合同只核对代码接口，不宣称数据当前全部新鲜可用。需要新研究时进入原研究中心，不新增Agent执行器或自动付费调用。

## 2026-10-15 组件调整兼容合同

- schema 仍为1，历史JSON不迁移、不回写。Style新增可选 `scale_content`、`scale_base_w`、`scale_base_h`、`show_source`；Instance新增可选 `refresh_seconds`。缺字段的旧配置保留原基础字号、底部来源和60秒检查行为。新系统默认开启内容联动、隐藏底部来源，已有用户默认/预设不追改。
- 四角等比缩放；四边单轴调整。`lock_ratio=true` 优先，四边与数值输入也按比例。网格先处理，比例和边界最后共同限制；碰撞阻止重叠。内部使用持久化基准尺寸，基础字号不被缩放写回；单轴更新基准以保留当前文字比例。拖动中预览，松手更新草稿。
- `show_source=false` 释放底部来源行；标题时间详情保留逐项来源。缓存、部分数据、错误、条件和缺口不受该开关隐藏。行情不使用 fetched_at 或请求完成时间冒充源时间；emotion只用自身交易日，themes用各项交易日，混合指数/自选显示各项时间不同。研究报告分开提供生成时间和原证据时间，未知保持未知。
- `refresh_seconds` 缺失=60，0=关闭，正整数范围60至86400秒。复制数据参数时复制间隔，重置数据参数时恢复所选默认间隔；预设与默认保存整个实例快照。后端严格校验，同前端一致。
- 同一看板共享调度，同资源请求合并，各实例仅在自身到期时更新展示；隐藏页面暂停，返回补查。关闭仍允许首次载入与页面显式刷新；若后台更新未完成，仅有界检查最多2分钟，完成或超时后停止，超时保留旧数据并允许页面重试。常规失败保留旧值/源时间并退避。同源后台补取不重拉无关AI档案。
- 卡片没有刷新按钮和点击标题刷新入口。页面级刷新保留；AI刷新只GET已有结果/任务状态，固定绑定不会自动切换，不启动模型/Agent/Skill分析。

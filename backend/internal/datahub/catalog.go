// Package datahub defines source identity, field contracts and observed health.
// A transport (HTTP/MCP) is not an independent upstream source.
package datahub

type Source struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Category string `json:"category"`
	Role     string `json:"role"`
}

func Sources() []Source {
	return []Source{
		{"tushare", "Tushare", "行情、历史、分钟、财务", "primary"},
		{"eastmoney", "东方财富", "行情、概念、公告、研报", "primary"},
		{"sina", "新浪财经", "行情、资金、财务、研报目录与纳指100备用", "backup"},
		{"tencent", "腾讯财经", "行情、指数、行业", "backup"},
		{"cls", "财联社", "市场资讯", "specialist"},
		{"duanxianxia", "短线侠 / 开盘啦", "题材、连板", "specialist"},
		{"ths", "同花顺", "人气榜", "specialist"},
		{"cffex", "中金所", "股指期货持仓", "backup"},
		{"cninfo", "巨潮资讯", "官方公告目录与 PDF 原文链接", "backup"},
	}
}

type Candidate struct {
	Family     string `json:"family"`
	Channel    string `json:"channel"`
	State      string `json:"state"`
	Equivalent bool   `json:"equivalent"`
	Note       string `json:"note,omitempty"`
}
type Contract struct {
	ID        string      `json:"id"`
	Name      string      `json:"name"`
	Unit      string      `json:"unit"`
	Universe  string      `json:"universe"`
	Frequency string      `json:"frequency"`
	Primary   Candidate   `json:"primary"`
	Backups   []Candidate `json:"backups"`
	Rule      string      `json:"rule"`
}

func current(family string) Candidate {
	return Candidate{Family: family, Channel: "http", State: "implemented", Equivalent: true}
}
func pending(family, note string) Candidate {
	return Candidate{Family: family, Channel: "http", State: "pending", Note: note}
}
func degraded(family, note string) Candidate {
	return Candidate{Family: family, Channel: "http", State: "degraded", Note: note}
}

// States describe implementation, not health; only live probes establish availability.
func Contracts() []Contract {
	return []Contract{
		{"quote", "个股实时行情", "CNY; shares; CNY turnover", "单一证券", "session", current("tushare"), []Candidate{current("sina"), current("tencent")}, "价格、数量、金额单位分别定义；时间戳过期不视为实时"},
		{"kline", "个股日 K", "CNY; lots; CNY turnover when supplied", "单一证券，前复权", "daily", current("eastmoney"), []Candidate{degraded("tencent", "同基准 OHLC/量；不提供成交额和换手率"), degraded("tushare", "日/周/月整段前复权备用；只含完成日线、厂商算法差异明示，不提供分钟或换手率"), degraded("sina", "当前接口未声明同一复权基准，不用于自动价格切换")}, "腾讯可替代同基准价格序列，缺少字段必须标识；禁止未复权拼接"},
		{"adjusted_kline", "复权价格", "CNY", "单一证券，指定复权基准", "daily", current("eastmoney"), []Candidate{degraded("tushare", "daily+adj_factor已接入日/周/月整段前复权，最新因子基准明示；与东财算法存在数值差异，不能逐点拼接"), degraded("tencent", "已有显式qfq价格备用，量额字段按实际供应保留缺失")}, "禁止未复权或不同基准序列自动拼接"},
		{"market_amount", "沪深 A 股成交额", "CNY", "上海 A 股 + 深圳 A 股；不含基金、债券、北交所", "daily/session", current("tushare"), []Candidate{current("eastmoney"), current("sina")}, "主源日线千元乘1000；新浪以完整48个5分钟成交额汇总日额，缺段不可用；000002.SH/399107.SZ，不以深证成指替代深市总额"},
		{"market_minutes", "沪深同时段分钟额", "CNY", "上海 A 股 + 深圳 A 股", "5min", current("tushare"), []Candidate{current("eastmoney"), current("sina")}, "Tushare→东方财富→新浪整组切换；新浪首桶含竞价，排除形成中分钟；逐来源保留失败，至少3日实时验收仍待完成"},
		{"indexes", "指数参照", "index points; percent", "各指数独立，不相加", "session", current("eastmoney"), []Candidate{current("tencent"), degraded("sina", "纳指100报价可能延迟至少15分钟；历史日/周/月OHLC独立备用，截止日期与量额缺口明示"), current("tushare")}, "指数唯一 ID；纳指100主报价为腾讯，备用新浪；禁止用纳指综合或期货替代，历史K线不以实时价格补齐缺失日期"},
		{"themes", "题材与成分", "membership; percent", "供应商题材版本", "session", current("duanxianxia"), []Candidate{degraded("eastmoney", "概念替代，不能当作同一分类"), degraded("tencent", "行业补充，不等于题材")}, "映射必须记录来源和版本；无法映射时显示不同分类"},
		{"limit_up", "涨跌停与连板", "count", "沪深非 ST；其他市场单列", "session/daily", current("duanxianxia"), []Candidate{current("eastmoney"), degraded("tushare", "日频统计，不冒充实时涨停池")}, "新股、ST、10/20/30% 和停牌规则一致；截断样本不能计总量"},
		{"breadth", "市场宽度", "count", "沪深 A 股；停牌不计涨跌", "session/daily", current("tushare"), []Candidate{current("eastmoney"), current("sina")}, "新浪沪深名单与科创板名单并集含CDR；剔除北交所和无成交证券；全分页/全批次完整、同日和时差校验后整组切换。缺页不补零，证券集合哈希及逐只采样可核验"},
		{"fund_flows", "资金动向", "CNY", "行业/题材/股票分别统计", "session/daily", current("sina"), []Candidate{pending("eastmoney", "按字段校验资金算法"), degraded("tushare", "个股日频，算法不同")}, "不同净流入算法保留来源，不静默拼成连续序列"},
		{"financials", "实际财报与估值", "CNY; ratio; percent", "单一证券、指定报表期", "disclosure", current("eastmoney"), []Candidate{current("sina"), pending("tushare", "fina_indicator 已测，待业务接入")}, "实际/预告分开；报告期、披露日和最新性均校验"},
		{"billboard", "龙虎榜", "CNY", "指定交易日和证券", "daily", current("eastmoney"), []Candidate{degraded("tushare", "top_list+top_inst已接入同日同原因榜单/席位；上榜原因条数可能不同，对侧金额与个别换手率保留缺失"), pending("exchange", "官方原始榜待接入")}, "席位别名另行记录，截断不能称完整榜"},
		{"margin", "两融", "CNY", "上海/深圳/北交所同日齐备汇总", "daily delayed", current("eastmoney"), []Candidate{degraded("tushare", "三市同日余额与融资流量已接入；融券偿还量未提供，未齐备日期不计全市场"), pending("exchange", "官方披露待接入")}, "正常披露延迟不是实时故障"},
		{"futures", "期货持仓", "contracts", "同一合约、同排名范围", "daily", current("eastmoney"), []Candidate{degraded("cffex", "已有单日快照备用，不提供完整历史/基差"), pending("tushare", "fut_holding 按合约接入")}, "部分字段空值保留，不能伪造连续历史"},
		{"news", "市场快讯", "document", "公开资讯", "publication", current("cls"), []Candidate{current("eastmoney")}, "同一转载不算两条独立证据，保留原文及发布时间"},
		{"announcements", "公告", "document", "指定证券、披露时点", "publication", current("eastmoney"), []Candidate{degraded("cninfo", "已接入证券/标题/日期/PDF链接；分类映射和PDF正文尚不等价"), pending("exchange", "官方原文待接入")}, "同源 MCP/HTTP 仅通道互备，片段按文档去重"},
		{"reports", "研报", "document", "指定对象、作者和日期", "publication", current("eastmoney"), []Candidate{degraded("sina", "独立公开目录：标题/机构/作者/日期/原文链接；不提供同口径评级、目标价、EPS/PE或完整PDF，分类不静默映射"), degraded("tushare", "已授权研报元数据/摘要；PDF可能同源东方财富，仅算通道备用")}, "元数据备用可用不等于完整覆盖；相同机构原文跨平台不计作独立观点，缺字段保持缺失"},
		{"hot_rank", "人气排名", "rank", "各供应商独立榜单", "hourly", current("ths"), []Candidate{degraded("eastmoney", "独立榜单，名次不可混合")}, "两榜分列，保留各自的排名历史"},
		{"opinions", "作者观点", "document", "同一作者原文", "publication", current("original"), []Candidate{pending("author_mirror", "需核实同作者同步账号")}, "微信、雪球、淘股吧为受限导入渠道，不以另一作者替代"},
		{"knowledge", "策略知识", "versioned document", "已授权知识与用户笔记", "version", current("github"), []Candidate{degraded("local_copy", "离线版本，不是独立实时来源")}, "来源更新不能覆盖用户笔记；知识不自动成为交易规则"},
	}
}

func ContentChannels() []string {
	return []string{"wechat", "xueqiu", "taoguba", "remote_daily_review", "github_knowledge"}
}

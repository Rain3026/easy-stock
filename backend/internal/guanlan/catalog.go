package guanlan

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strings"
	"unicode/utf8"
)

var IndexIDs = []string{"sse", "szse", "chinext", "csi300", "sse50", "csi1000", "star50", "hsi", "dow", "sp500", "nasdaq", "nasdaq100"}

func Catalog() []Definition {
	types := []struct {
		id, name, category, description, source string
		w, h, minW, minH                        int
		params                                  map[string]any
		keys                                    []string
	}{
		{"volume", "沪深量能", "行情", "实际与预计成交额、前20日均额与观察线", "行情缓存 · marketpulse", 72, 34, 36, 20, map[string]any{"history_days": 20, "show_forecast": true, "show_mean": true}, []string{"history_days", "show_forecast", "show_mean", "reference_line"}},
		{"indexes", "指数参照", "行情", "单个或多个指数，独立选择显示字段", "行情缓存 · core indexes", 120, 16, 20, 10, map[string]any{"index_ids": []string{"sse", "szse", "chinext", "csi300"}, "mode": "horizontal", "fields": []string{"price", "change", "time"}}, []string{"index_ids", "mode", "fields"}},
		{"breadth", "市场宽度", "行情", "同口径涨跌平与市场广度", "行情缓存 · breadth", 48, 22, 20, 10, map[string]any{"fields": []string{"up", "down", "flat", "percent"}}, []string{"fields"}},
		{"emotion", "涨跌停与连板", "行情", "涨跌停家数、最高连板及梯队", "行情缓存 · emotion", 48, 24, 28, 16, map[string]any{"rows": 5}, []string{"rows"}},
		{"themes", "核心主线", "行情", "主线热度、涨幅与持续性", "行情缓存 · themes", 60, 24, 32, 16, map[string]any{"rows": 5, "sort": "strength"}, []string{"rows", "sort"}},
		{"debate", "多空辨析", "AI研究", "已有多空辩论结论和证据引用", "研究档案 · Agent / Skill", 48, 30, 32, 18, map[string]any{}, []string{"snapshot_id", "result_id", "subject"}},
		{"risk", "持仓风险", "AI研究", "只读已有组合巡检与风险摘要", "研究档案 · portfolio", 48, 26, 32, 18, map[string]any{}, []string{"plan_id", "result_id"}},
		{"watchlist", "自选观察", "关注", "共享关注名单与实时行情", "共享名单 · quote cache", 60, 24, 32, 16, map[string]any{"fields": []string{"price", "change", "time"}, "sort": "manual"}, []string{"list_id", "fields", "sort"}},
	}
	result := make([]Definition, 0, len(types))
	for _, t := range types {
		showSource := false
		style := Style{ScaleContent: true, ScaleBaseW: float64(t.w), ScaleBaseH: float64(t.h), ShowSource: &showSource, Title: t.name, TitleSize: 16, ValueSize: 28, TextSize: 12, HeaderHeight: 44, ChartHeight: 220, Padding: 16, Gap: 12, Density: "standard", Align: "left", RowHeight: 34, RowCount: 8, ColumnWidth: 120}
		if t.id == "watchlist" {
			style.ValueSize = 16
			style.ColumnWidth = 92
			style.RowCount = 5
		}
		if t.id == "themes" {
			style.ValueSize = 16
		}
		if t.id == "debate" || t.id == "risk" {
			style.ValueSize = 18
		}
		result = append(result, Definition{ID: t.id, Name: t.name, Category: t.category, Description: t.description, Source: t.source, MinW: t.minW, MinH: t.minH, Default: Instance{Type: t.id, Schema: Schema, Layout: Layout{W: float64(t.w), H: float64(t.h)}, Style: style, Params: t.params}, Params: t.keys})
	}
	return result
}
func Find(id string) (Definition, bool) {
	for _, d := range Catalog() {
		if d.ID == id {
			return d, true
		}
	}
	return Definition{}, false
}
func Clone[T any](v T) T { b, _ := json.Marshal(v); var out T; _ = json.Unmarshal(b, &out); return out }
func validName(name string) bool {
	return strings.TrimSpace(name) != "" && utf8.RuneCountInString(name) <= 80
}
func ValidateConfig(c Config) error {
	if c.Schema != Schema || !validName(c.Name) || c.Items == nil || len(c.Items) > 60 {
		return errors.New("配置版本、名称或组件数量无效（最多60个）")
	}
	ids := map[string]bool{}
	for _, i := range c.Items {
		if i.ID == "" || len(i.ID) > 100 || ids[i.ID] {
			return errors.New("组件实例标识缺失或重复")
		}
		ids[i.ID] = true
		if e := ValidateInstance(i); e != nil {
			return e
		}
	}
	return nil
}
func ValidateInstance(i Instance) error {
	if i.RefreshSeconds != nil && (*i.RefreshSeconds < 0 || (*i.RefreshSeconds > 0 && *i.RefreshSeconds < 60) || *i.RefreshSeconds > 86400) {
		return errors.New("自动刷新间隔须为0（关闭）或60至86400秒")
	}
	for _, n := range []float64{i.Style.ScaleBaseW, i.Style.ScaleBaseH} {
		if math.IsNaN(n) || math.IsInf(n, 0) || n < 0 || n > 100000 {
			return errors.New("内容缩放基准无效")
		}
	}
	if (i.Style.ScaleBaseW == 0) != (i.Style.ScaleBaseH == 0) {
		return errors.New("内容缩放基准须同时提供宽高")
	}

	if i.Style.RowCount < 0 || i.Style.RowCount > 100 {
		return errors.New("表格显示行数无效")
	}
	d, ok := Find(i.Type)
	if !ok || i.Schema != Schema {
		return errors.New("组件类型或版本不受支持，请保留原版本并复制修正")
	}
	l, s := i.Layout, i.Style
	for _, n := range []float64{l.X, l.Y, l.W, l.H} {
		if math.IsNaN(n) || math.IsInf(n, 0) {
			return errors.New("组件尺寸必须为有限数字")
		}
	}
	if l.X < 0 || l.Y < 0 || l.Y > 10000 || l.W+1e-7 < float64(d.MinW) || l.X+l.W > 120+1e-7 || l.H+1e-7 < float64(d.MinH) || l.H > 200 {
		return errors.New("组件位置或大小超出可读范围")
	}
	if utf8.RuneCountInString(s.Title) > 80 || s.TitleSize < 10 || s.TitleSize > 48 || s.ValueSize < 12 || s.ValueSize > 80 || s.TextSize < 10 || s.TextSize > 28 || s.HeaderHeight < 32 || s.HeaderHeight > 160 || s.ChartHeight < 100 || s.ChartHeight > 1200 || s.Padding < 0 || s.Padding > 48 || s.Gap < 0 || s.Gap > 40 || s.RowHeight < 24 || s.RowHeight > 100 || s.ColumnWidth < 60 || s.ColumnWidth > 400 {
		return errors.New("组件样式超出可读范围")
	}
	if s.Columns != 0 && s.Columns != 2 && s.Columns != 4 {
		return errors.New("内部列数无效")
	}
	if !contains([]string{"compact", "standard", "loose"}, s.Density) || !contains([]string{"left", "center", "right"}, s.Align) {
		return errors.New("密度或对齐无效")
	}
	if i.Params == nil {
		return errors.New("组件参数缺失")
	}
	for key, v := range i.Params {
		if !contains(d.Params, key) {
			return fmt.Errorf("%s不支持参数%s", d.Name, key)
		}
		switch key {
		case "index_ids", "fields":
			b, _ := json.Marshal(v)
			var values []string
			if json.Unmarshal(b, &values) != nil || len(values) == 0 || len(values) > 12 {
				return errors.New("请选择有效字段/指数")
			}
			seen := map[string]bool{}
			for _, v := range values {
				allowed := []string{"price", "change", "time"}
				if key == "index_ids" {
					allowed = IndexIDs
				} else if i.Type == "breadth" {
					allowed = []string{"up", "down", "flat", "percent"}
				}
				if !contains(allowed, v) || seen[v] {
					return errors.New("指数/字段无效或重复")
				}
				seen[v] = true
			}
		case "show_forecast", "show_mean":
			if _, ok := v.(bool); !ok {
				return errors.New("开关参数须为布尔值")
			}
		case "rows", "history_days", "reference_line":
			n, ok := number(v)
			max, min := 20.0, 1.0
			if key == "reference_line" {
				max, min = 1000000, 100
			}
			if !ok || n < min || n > max || math.IsNaN(n) || math.IsInf(n, 0) || math.Trunc(n) != n {
				return errors.New("数字参数超出范围")
			}
		case "mode":
			if !contains([]string{"card", "horizontal", "list"}, fmt.Sprint(v)) {
				return errors.New("指数显示方式无效")
			}
		case "sort":
			allowed := []string{"strength", "change"}
			if i.Type == "watchlist" {
				allowed = []string{"manual", "change"}
			}
			if !contains(allowed, fmt.Sprint(v)) {
				return errors.New("排序方式无效")
			}
		default:
			value, ok := v.(string)
			if !ok || len(value) > 200 {
				return errors.New("结果或名单标识无效")
			}
		}
	}
	return nil
}
func number(v any) (float64, bool) {
	switch n := v.(type) {
	case float64:
		return n, true
	case int:
		return float64(n), true
	case json.Number:
		f, e := n.Float64()
		return f, e == nil
	}
	return 0, false
}
func contains(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}

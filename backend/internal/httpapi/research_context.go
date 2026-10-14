package httpapi

import (
	dr "easy-stock/backend/internal/dailyresearch"
	"easy-stock/backend/internal/datahub"
	"easy-stock/backend/internal/foundation"
	"encoding/json"
	"net/http"
	"time"
)

// Creates a reference to an existing, fixed record. It fetches no new market
// data and invokes no model; supplementary evidence always creates a child.
func (s *Server) researchContext(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Kind        string   `json:"kind"`
		RecordID    string   `json:"record_id"`
		EvidenceIDs []string `json:"evidence_ids"`
		ParentID    string   `json:"parent_id,omitempty"`
	}
	if !skillBody(w, r, &in) {
		return
	}
	v := dr.Snapshot{ID: dr.NewID(), Version: "research-context-v1", Status: "partial", StartedAt: time.Now().UTC(), Themes: []dr.Theme{}, Indexes: []foundation.MarketIndexSnapshot{}, Baselines: map[string]foundation.KLine{}, Gaps: []string{}}
	var payload any
	entity := ""
	refs := []datahub.EvidenceSource{}
	ids := []string{}
	switch in.Kind {
	case "stock":
		j, e := s.stockResearchStore.Get(r.Context(), in.RecordID)
		if e != nil || j.Snapshot == nil {
			writeError(w, 404, "原个股证据不可用")
			return
		}
		snapshot := *j.Snapshot
		if e = s.freezeStockEvidence(r.Context(), &snapshot); e != nil {
			writeError(w, 500, "保存原始证据失败")
			return
		}
		ids = append(ids, snapshot.EvidenceID)
		v.Cutoff = snapshot.CutoffAt
		entity = "stock:" + snapshot.Symbol
		v.ContextLabel = snapshot.Name + " · " + snapshot.Symbol
		for _, bar := range snapshot.DailyBars {
			at, e := time.ParseInLocation("2006-01-02 15:04", bar.Date+" 15:00", foundation.AStockLocation)
			if e == nil && !at.After(v.Cutoff) && bar.Close > 0 {
				prior := v.Baselines[snapshot.Symbol]
				if at.After(prior.Time) {
					v.Baselines[snapshot.Symbol] = foundation.KLine{Symbol: snapshot.Symbol, Time: at, Open: bar.Open, Close: bar.Close, High: bar.High, Low: bar.Low, Volume: bar.Volume, Amount: bar.Amount, Meta: snapshot.Quote.Meta}
				}
			}
		}
		snapshot.Sources = append(snapshot.Sources[:0:0], snapshot.Sources...)
		for i := range snapshot.Sources {
			src := &snapshot.Sources[i]
			if len([]rune(src.Content)) > 1200 {
				src.Content = string([]rune(src.Content)[:1200]) + "…[上下文节选，完整原文见证据包]"
			}
			refs = append(refs, datahub.EvidenceSource{ID: src.ID, Family: src.Provider, URL: src.URL, CapturedAt: src.CapturedAt})
		}
		payload = snapshot
		v.Gaps = append(v.Gaps, snapshot.Limitations...)
	case "portfolio":
		j, e := s.portfolioInspection.Get(r.Context(), in.RecordID)
		if e != nil || j.Report == nil {
			writeError(w, 404, "原组合报告不可用")
			return
		}
		entity = "portfolio:" + j.Request.PortfolioPlanID
		if j.Request.PortfolioPlanID == "" {
			entity = "portfolio:report-" + j.ID
		}
		v.ContextLabel = j.Request.PortfolioPlanName
		if v.ContextLabel == "" {
			v.ContextLabel = "组合研究"
		}
		v.Cutoff = j.Report.GeneratedAt
		payload = map[string]any{"request": j.Request, "metrics": j.Report.Metrics, "facts": j.Report.Facts, "previous_ai_judgement": j.Report.Conclusion, "report_id": j.ID}
		v.Gaps = append(v.Gaps, "组合包含既有 AI 判断，须与事实和确定性指标区分；本次不补采行情、不发出交易指令", "未建立组合净值基准，不能将个股涨幅直接当作组合收益")
	case "evidence":
		if len(in.EvidenceIDs) == 0 || len(in.EvidenceIDs) > 10 {
			writeError(w, 400, "请选择 1 到 10 份同一对象的证据")
			return
		}
		items := []datahub.FactSnapshot{}
		for _, id := range in.EvidenceIDs {
			f, e := s.dailyResearch.store.Facts().Get(r.Context(), id)
			if e != nil || entity != "" && entity != f.EntityID {
				writeError(w, 400, "证据不存在或对象不一致")
				return
			}
			entity = f.EntityID
			if f.CutoffAt.After(v.Cutoff) {
				v.Cutoff = f.CutoffAt
			}
			items = append(items, f)
			refs = append(refs, f.Sources...)
			v.Gaps = append(v.Gaps, f.Gaps...)
			ids = append(ids, f.ID)
		}
		payload = items
		v.ContextLabel = entity
	default:
		writeError(w, 400, "不支持的研究上下文")
		return
	}
	if v.Cutoff.IsZero() || v.Cutoff.After(time.Now().Add(time.Minute)) {
		writeError(w, 400, "原记录缺少可靠截止时间")
		return
	}
	v.EntityID = entity
	v.TradeDate = v.Cutoff.In(foundation.AStockLocation).Format("2006-01-02")
	v.ContextEvidenceIDs = ids
	v.ContextData, _ = json.Marshal(payload)
	if len(v.ContextData) > 192<<10 {
		writeError(w, 413, "证据上下文过大，请选择较少证据后重试")
		return
	}
	parentFactID := ""
	if in.ParentID != "" {
		parent, e := s.dailyResearch.store.Snapshot(r.Context(), in.ParentID)
		parentEntity := parent.EntityID
		if parentEntity == "" {
			parentEntity = "market:cn"
		}
		if e != nil || parentEntity != entity {
			writeError(w, 400, "补证必须属于同一研究对象")
			return
		}
		v.ParentID = parent.ID
		parentFactID = parent.EvidenceID
		if parent.Cutoff.After(v.Cutoff) {
			v.Cutoff = parent.Cutoff
		}
		var previous any = parent.ContextData
		if len(parent.ContextData) == 0 {
			previous = parent
		}
		v.ContextData, _ = json.Marshal(map[string]any{"previous_evidence": previous, "supplement": v.ContextData})
		v.ContextEvidenceIDs = append(v.ContextEvidenceIDs, parent.ContextEvidenceIDs...)
		v.ContextEvidenceIDs = append(v.ContextEvidenceIDs, parent.EvidenceID)
		for symbol, bar := range parent.Baselines {
			if _, ok := v.Baselines[symbol]; !ok {
				v.Baselines[symbol] = bar
			}
		}

		v.Gaps = append(v.Gaps, "补证产生新快照，旧判断未改写，须重新辨析")
	}
	if len(v.ContextData) > 192<<10 {
		writeError(w, 413, "补证后上下文过大，请缩小范围")
		return
	}
	v.StartedAt = v.Cutoff
	raw, _ := json.Marshal(v)
	f, e := s.dailyResearch.store.Facts().Freeze(r.Context(), datahub.FactSnapshot{EntityID: entity, Kind: "context-evidence", OriginID: in.RecordID, ParentID: parentFactID, MethodVersion: v.Version, CapturedAt: time.Now().UTC(), CutoffAt: v.Cutoff, Sources: refs, Gaps: v.Gaps, Payload: raw})
	if e != nil {
		writeError(w, 500, "上下文证据保存失败")
		return
	}
	v.EvidenceID = f.ID
	if e = s.dailyResearch.store.Append(r.Context(), v.ID, "snapshot", v.ID, v); e != nil {
		writeError(w, 500, "研究快照保存失败")
		return
	}
	writeJSON(w, 201, map[string]any{"data": v, "subject": entity})
}

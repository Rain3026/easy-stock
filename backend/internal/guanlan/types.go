// Package guanlan owns display configurations, never market or research facts.
package guanlan

import "time"

const Schema = 1

type Layout struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
	W float64 `json:"w"`
	H float64 `json:"h"`
}
type Style struct {
	Title        string  `json:"title"`
	TitleSize    int     `json:"title_size"`
	ValueSize    int     `json:"value_size"`
	TextSize     int     `json:"text_size"`
	HeaderHeight int     `json:"header_height"`
	ChartHeight  int     `json:"chart_height"`
	Padding      int     `json:"padding"`
	Gap          int     `json:"gap"`
	Columns      int     `json:"columns"`
	Density      string  `json:"density"`
	Align        string  `json:"align"`
	RowHeight    int     `json:"row_height"`
	RowCount     int     `json:"row_count,omitempty"`
	ColumnWidth  int     `json:"column_width"`
	LockRatio    bool    `json:"lock_ratio"`
	ScaleContent bool    `json:"scale_content,omitempty"`
	ScaleBaseW   float64 `json:"scale_base_w,omitempty"`
	ScaleBaseH   float64 `json:"scale_base_h,omitempty"`
	ShowSource   *bool   `json:"show_source,omitempty"`
}
type Instance struct {
	ID             string         `json:"id"`
	Type           string         `json:"type"`
	Schema         int            `json:"schema"`
	Layout         Layout         `json:"layout"`
	Style          Style          `json:"style"`
	Params         map[string]any `json:"params"`
	RefreshSeconds *int           `json:"refresh_seconds,omitempty"`
}
type Config struct {
	Schema int        `json:"schema"`
	Name   string     `json:"name"`
	Snap   bool       `json:"snap"`
	Items  []Instance `json:"items"`
}
type Draft struct {
	ID        string    `json:"id"`
	Revision  int64     `json:"revision"`
	SourceID  string    `json:"source_id"`
	Config    Config    `json:"config"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}
type Version struct {
	ID        string    `json:"id"`
	Number    int64     `json:"number"`
	Name      string    `json:"name"`
	SourceID  string    `json:"source_id"`
	CreatedAt time.Time `json:"created_at"`
	Hash      string    `json:"hash"`
	Config    Config    `json:"config"`
	Archived  bool      `json:"archived"`
	Revision  int64     `json:"revision"`
}
type Active struct {
	VersionID string `json:"version_id"`
	Revision  int64  `json:"revision"`
}
type Preference struct {
	Favorite bool      `json:"favorite"`
	Hidden   bool      `json:"hidden"`
	Default  *Instance `json:"default,omitempty"`
}
type Preset struct {
	ID       string   `json:"id"`
	Name     string   `json:"name"`
	Revision int64    `json:"revision"`
	Item     Instance `json:"item"`
}
type Watchlist struct {
	ID       string   `json:"id"`
	Name     string   `json:"name"`
	Revision int64    `json:"revision"`
	Symbols  []string `json:"symbols"`
}
type State struct {
	Revision        int64                 `json:"revision"`
	LibraryRevision int64                 `json:"library_revision"`
	Active          Active                `json:"active"`
	Drafts          map[string]Draft      `json:"drafts"`
	Library         map[string]Preference `json:"library"`
	Presets         map[string]Preset     `json:"presets"`
	Watchlists      map[string]Watchlist  `json:"watchlists"`
}
type Definition struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Category    string   `json:"category"`
	Description string   `json:"description"`
	Source      string   `json:"source"`
	MinW        int      `json:"min_w"`
	MinH        int      `json:"min_h"`
	Default     Instance `json:"default"`
	Params      []string `json:"params"`
}
type Page struct {
	Items    []Version `json:"items"`
	Total    int       `json:"total"`
	Page     int       `json:"page"`
	PageSize int       `json:"page_size"`
}
type SaveResult struct {
	Version Version `json:"version"`
	Active  Active  `json:"active"`
}

// Package pagination provides a uniform offset/limit pagination contract
// for list endpoints. It enforces validation on the wire (page >= 1,
// 1 <= limit <= MaxLimit) and produces a single response envelope so
// every list endpoint looks the same to clients.
package pagination

import (
	"strconv"

	"github.com/gofiber/fiber/v2"
)

const (
	DefaultPage  = 1
	DefaultLimit = 20
	MaxLimit     = 100
)

// Params holds normalized page/limit values.
type Params struct {
	Page  int
	Limit int
}

// Offset converts page/limit to SQL OFFSET.
func (p Params) Offset() int {
	return (p.Page - 1) * p.Limit
}

// FromCtx parses ?page=&limit= from the request, clamps them to safe
// ranges, and applies defaults. It never returns an error — malformed
// input falls back to defaults so clients get a list instead of a 400.
func FromCtx(c *fiber.Ctx) Params {
	page := parseIntOrDefault(c.Query("page"), DefaultPage)
	limit := parseIntOrDefault(c.Query("limit"), DefaultLimit)

	if page < 1 {
		page = DefaultPage
	}
	if limit < 1 {
		limit = DefaultLimit
	}
	if limit > MaxLimit {
		limit = MaxLimit
	}

	return Params{Page: page, Limit: limit}
}

func parseIntOrDefault(s string, def int) int {
	if s == "" {
		return def
	}
	n, err := strconv.Atoi(s)
	if err != nil {
		return def
	}
	return n
}

// Meta is the pagination metadata returned alongside items.
type Meta struct {
	Page       int   `json:"page"`
	Limit      int   `json:"limit"`
	Total      int64 `json:"total"`
	TotalPages int   `json:"total_pages"`
	HasNext    bool  `json:"has_next"`
	HasPrev    bool  `json:"has_prev"`
}

// BuildMeta assembles Meta from params + total row count.
func BuildMeta(p Params, total int64) Meta {
	totalPages := 0
	if p.Limit > 0 {
		totalPages = int((total + int64(p.Limit) - 1) / int64(p.Limit))
	}
	return Meta{
		Page:       p.Page,
		Limit:      p.Limit,
		Total:      total,
		TotalPages: totalPages,
		HasNext:    p.Page < totalPages,
		HasPrev:    p.Page > 1,
	}
}

// Response is the canonical envelope for paginated list endpoints.
// Items is kept as interface{} so handlers can pass any concrete slice
// without generics leaking into the wire format.
type Response struct {
	Items      interface{} `json:"items"`
	Pagination Meta        `json:"pagination"`
}

// Build wraps items + meta into the envelope.
func Build(items interface{}, p Params, total int64) Response {
	return Response{
		Items:      items,
		Pagination: BuildMeta(p, total),
	}
}

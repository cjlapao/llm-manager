package api

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"
)

// SortDirection represents the direction of a sort field.
type SortDirection string

const (
	SortASC  SortDirection = "ASC"
	SortDESC SortDirection = "DESC"
)

// ODataQuery represents the parsed and validated OData-style query parameters.
// Zero values mean "no filtering" — every field is optional.
type ODataQuery struct {
	Page      int
	Limit     int
	SortField string
	SortDir   SortDirection
	Filter    map[string]string
	Search    string
	Fields    []string
}

// ParseODataQuery parses and validates OData-style query parameters from an
// incoming HTTP request. All parameters are optional; a zero-value ODataQuery
// means "no filtering requested."
//
// Supported query params:
//
//	?page=1          — page number (1-based, default 1)
//	?limit=25        — results per page (default 25, max 500)
//	?sort=-name      — sort field with optional '-' prefix for descending
//	?filter=type:llm,status:running  — comma-separated key:value pairs
//	?search=foo      — case-insensitive search term
//	?fields=name,sub_type  — comma-separated field names to project
func ParseODataQuery(r *http.Request) (ODataQuery, error) {
	q := r.URL.Query()
	var errs []string

	qry := ODataQuery{
		Page:  1,
		Limit: 25,
	}

	// --- page ---
	if v := q.Get("page"); v != "" {
		p, err := strconv.Atoi(v)
		if err != nil || p < 1 {
			errs = append(errs, "page must be a positive integer")
		} else {
			qry.Page = p
		}
	}

	// --- limit ---
	if v := q.Get("limit"); v != "" {
		l, err := strconv.Atoi(v)
		if err != nil || l < 1 {
			errs = append(errs, "limit must be a positive integer")
		} else if l > 500 {
			errs = append(errs, "limit must not exceed 500")
		} else {
			qry.Limit = l
		}
	}

	// --- sort ---
	if v := q.Get("sort"); v != "" {
		dir := SortASC
		field := v
		if strings.HasPrefix(v, "-") {
			dir = SortDESC
			field = strings.TrimPrefix(v, "-")
		}
		if field == "" {
			errs = append(errs, "sort field must not be empty")
		} else if !isValidSortField(field) {
			errs = append(errs, fmt.Sprintf("sort field %q contains invalid characters (use letters, digits, underscore, hyphen)", field))
		} else {
			qry.SortField = field
			qry.SortDir = dir
		}
	}

	// --- filter ---
	if v := q.Get("filter"); v != "" {
		fmap, err := parseFilter(v)
		if err != nil {
			errs = append(errs, err.Error())
		} else {
			qry.Filter = fmap
		}
	}

	// --- search ---
	qry.Search = q.Get("search")

	// --- fields ---
	if q.Has("fields") {
		fields := parseFields(q.Get("fields"))
		if len(fields) == 0 {
			errs = append(errs, "fields parameter must contain at least one field name")
		} else {
			qry.Fields = fields
		}
	}

	if len(errs) > 0 {
		return ODataQuery{}, fmt.Errorf("invalid query parameters: %s", strings.Join(errs, "; "))
	}

	return qry, nil
}

// isValidSortField checks that a sort field contains only allowed characters.
func isValidSortField(f string) bool {
	for _, c := range f {
		if !((c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') ||
			(c >= '0' && c <= '9') || c == '_' || c == '-') {
			return false
		}
	}
	return true
}

// parseFilter parses a comma-separated list of key:value pairs into a map.
// Example: "type:llm,status:running" → map[string]string{"type":"llm","status":"running"}
func parseFilter(s string) (map[string]string, error) {
	result := make(map[string]string)
	pairs := strings.Split(s, ",")
	for _, pair := range pairs {
		pair = strings.TrimSpace(pair)
		if pair == "" {
			continue
		}
		idx := strings.Index(pair, ":")
		if idx <= 0 {
			return nil, fmt.Errorf("malformed filter pair %q (expected key:value)", pair)
		}
		key := strings.TrimSpace(pair[:idx])
		value := strings.TrimSpace(pair[idx+1:])
		if key == "" {
			return nil, fmt.Errorf("malformed filter pair %q (empty key)", pair)
		}
		result[key] = value
	}
	return result, nil
}

// parseFields splits a comma-separated field list, trimming whitespace and
// filtering out empty entries.
func parseFields(s string) []string {
	raw := strings.Split(s, ",")
	fields := make([]string, 0, len(raw))
	for _, f := range raw {
		f = strings.TrimSpace(f)
		if f != "" {
			fields = append(fields, f)
		}
	}
	return fields
}

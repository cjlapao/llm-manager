// Package api provides the HTTP API server for llm-manager.
package api

import (
	"fmt"

	"gorm.io/gorm"
)

// ODataOptions carries the parsed OData query parameters plus model metadata
// needed for GORM query modification.
type ODataOptions struct {
	// ODataQuery is the parsed and validated query parameters from Task #53.
	ODataQuery

	// ModelTable is the GORM table name (e.g., "models", "containers").
	ModelTable string

	// ModelType is the Go type of the model being queried (used for Count).
	ModelType interface{}
}

// ApplyODataFilters applies pagination, sorting, filtering, search, and field
// projection to a GORM query. It returns the modified *gorm.DB, the total
// count of matching records (before pagination), and any error.
//
// The function:
//  1. Applies sort, filter, and search to a count query to get total matches.
//  2. Applies the same sort, filter, and search to the result query.
//  3. Applies pagination (LIMIT/OFFSET) to the result query.
//  4. Applies field projection (SELECT fields) to the result query.
func ApplyODataFilters(db *gorm.DB, opts ODataOptions) (*gorm.DB, int64, error) {
	// Build the base query with sort, filter, and search applied.
	// Both the count query and the result query share these modifiers.
	baseQuery := db

	// --- Sort ---
	if opts.SortField != "" {
		// Validate sort column against white-list
		validated, err := ValidateFields(opts.ModelTable, []string{opts.SortField})
		if err != nil {
			return nil, 0, fmt.Errorf("sort field %q: %w", opts.SortField, err)
		}
		orderCol := validated[0]
		if opts.SortDir == SortDESC {
			orderCol = opts.SortField + " DESC"
		}
		baseQuery = baseQuery.Order(orderCol)
	}

	// --- Filter ---
	if len(opts.Filter) > 0 {
		// Validate all filter columns
		filterCols := make([]string, 0, len(opts.Filter))
		for col := range opts.Filter {
			filterCols = append(filterCols, col)
		}
		validated, err := ValidateFields(opts.ModelTable, filterCols)
		if err != nil {
			return nil, 0, fmt.Errorf("filter fields: %w", err)
		}
		// Rebuild validated map
		validatedFilter := make(map[string]string, len(validated))
		for _, col := range validated {
			validatedFilter[col] = opts.Filter[col]
		}
		for col, val := range validatedFilter {
			baseQuery = baseQuery.Where(col+" = ?", val)
		}
	}

	// --- Search ---
	if opts.Search != "" {
		searchTerm := "%" + opts.Search + "%"
		baseQuery = baseQuery.Where("name LIKE ? OR slug LIKE ?", searchTerm, searchTerm)
	}

	// --- Count ---
	var total int64
	if opts.ModelType != nil {
		if err := baseQuery.Model(opts.ModelType).Count(&total).Error; err != nil {
			return nil, 0, fmt.Errorf("failed to count records: %w", err)
		}
	} else {
		if err := baseQuery.Count(&total).Error; err != nil {
			return nil, 0, fmt.Errorf("failed to count records: %w", err)
		}
	}

	// --- Pagination ---
	if opts.Limit > 0 {
		offset := (opts.Page - 1) * opts.Limit
		db = db.Limit(opts.Limit).Offset(offset)
	}

	// --- Field Projection ---
	if len(opts.Fields) > 0 {
		// Validate fields against white-list
		validated, err := ValidateFields(opts.ModelTable, opts.Fields)
		if err != nil {
			return nil, 0, fmt.Errorf("field projection: %w", err)
		}
		db = ApplyFieldProjection(db, validated)
	}

	return db, total, nil
}

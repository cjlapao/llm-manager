package api

import (
	"net/http"
	"strings"
	"testing"
)

func newTestRequest(query string) *http.Request {
	req, err := http.NewRequest(http.MethodGet, "http://localhost"+query, nil)
	if err != nil {
		panic("failed to create request: " + err.Error())
	}
	return req
}

func TestParseODataQuery_Defaults(t *testing.T) {
	qry, err := ParseODataQuery(newTestRequest(""))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if qry.Page != 1 {
		t.Errorf("expected Page=1, got %d", qry.Page)
	}
	if qry.Limit != 25 {
		t.Errorf("expected Limit=25, got %d", qry.Limit)
	}
	if qry.SortField != "" {
		t.Errorf("expected empty SortField, got %q", qry.SortField)
	}
	if qry.SortDir != "" {
		t.Errorf("expected empty SortDir, got %q", qry.SortDir)
	}
	if qry.Filter != nil {
		t.Errorf("expected nil Filter, got %v", qry.Filter)
	}
	if qry.Search != "" {
		t.Errorf("expected empty Search, got %q", qry.Search)
	}
	if qry.Fields != nil {
		t.Errorf("expected nil Fields, got %v", qry.Fields)
	}
}

func TestParseODataQuery_ValidParams(t *testing.T) {
	tests := []struct {
		name     string
		query    string
		wantPage int
		wantLim  int
		wantSort string
		wantDir  SortDirection
		wantFilt map[string]string
		wantSrch string
		wantFlds []string
	}{
		{
			name:     "page only",
			query:    "?page=3",
			wantPage: 3,
			wantLim:  25,
		},
		{
			name:    "limit only",
			query:   "?limit=100",
			wantLim: 100,
		},
		{
			name:     "sort ascending",
			query:    "?sort=name",
			wantSort: "name",
			wantDir:  SortASC,
		},
		{
			name:     "sort descending",
			query:    "?sort=-name",
			wantSort: "name",
			wantDir:  SortDESC,
		},
		{
			name:     "sort descending with hyphenated field",
			query:    "?sort=-sub_type",
			wantSort: "sub_type",
			wantDir:  SortDESC,
		},
		{
			name:     "filter single pair",
			query:    "?filter=type:llm",
			wantFilt: map[string]string{"type": "llm"},
		},
		{
			name:     "filter multiple pairs",
			query:    "?filter=type:llm,status:running",
			wantFilt: map[string]string{"type": "llm", "status": "running"},
		},
		{
			name:     "filter with spaces around colons",
			query:    "?filter=type : llm , status : running",
			wantFilt: map[string]string{"type": "llm", "status": "running"},
		},
		{
			name:     "search",
			query:    "?search=foobar",
			wantSrch: "foobar",
		},
		{
			name:     "search case-insensitive preserved",
			query:    "?search=FooBar",
			wantSrch: "FooBar",
		},
		{
			name:     "fields single",
			query:    "?fields=name",
			wantFlds: []string{"name"},
		},
		{
			name:     "fields multiple",
			query:    "?fields=name,sub_type,provider",
			wantFlds: []string{"name", "sub_type", "provider"},
		},
		{
			name:     "fields with spaces",
			query:    "?fields= name , sub_type ",
			wantFlds: []string{"name", "sub_type"},
		},
		{
			name:     "all params combined",
			query:    "?page=2&limit=100&sort=-name&filter=type:llm,status:running&search=chat&fields=name,provider",
			wantPage: 2,
			wantLim:  100,
			wantSort: "name",
			wantDir:  SortDESC,
			wantFilt: map[string]string{"type": "llm", "status": "running"},
			wantSrch: "chat",
			wantFlds: []string{"name", "provider"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			qry, err := ParseODataQuery(newTestRequest(tc.query))
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tc.wantPage != 0 && qry.Page != tc.wantPage {
				t.Errorf("Page: want %d, got %d", tc.wantPage, qry.Page)
			}
			if tc.wantLim != 0 && qry.Limit != tc.wantLim {
				t.Errorf("Limit: want %d, got %d", tc.wantLim, qry.Limit)
			}
			if tc.wantSort != "" && qry.SortField != tc.wantSort {
				t.Errorf("SortField: want %q, got %q", tc.wantSort, qry.SortField)
			}
			if tc.wantDir != "" && qry.SortDir != tc.wantDir {
				t.Errorf("SortDir: want %q, got %q", tc.wantDir, qry.SortDir)
			}
			if tc.wantFilt != nil {
				if len(qry.Filter) != len(tc.wantFilt) {
					t.Errorf("Filter: want %v, got %v", tc.wantFilt, qry.Filter)
					return
				}
				for k, v := range tc.wantFilt {
					if qry.Filter[k] != v {
						t.Errorf("Filter[%q]: want %q, got %q", k, v, qry.Filter[k])
					}
				}
			}
			if tc.wantSrch != "" && qry.Search != tc.wantSrch {
				t.Errorf("Search: want %q, got %q", tc.wantSrch, qry.Search)
			}
			if tc.wantFlds != nil {
				if len(qry.Fields) != len(tc.wantFlds) {
					t.Errorf("Fields: want %v, got %v", tc.wantFlds, qry.Fields)
					return
				}
				for i, f := range tc.wantFlds {
					if qry.Fields[i] != f {
						t.Errorf("Fields[%d]: want %q, got %q", i, f, qry.Fields[i])
					}
				}
			}
		})
	}
}

func TestParseODataQuery_InvalidValues(t *testing.T) {
	tests := []struct {
		name    string
		query   string
		wantErr bool
	}{
		{name: "negative page", query: "?page=-1", wantErr: true},
		{name: "zero page", query: "?page=0", wantErr: true},
		{name: "non-integer page", query: "?page=abc", wantErr: true},
		{name: "negative limit", query: "?limit=-5", wantErr: true},
		{name: "zero limit", query: "?limit=0", wantErr: true},
		{name: "non-integer limit", query: "?limit=abc", wantErr: true},
		{name: "limit exceeds max", query: "?limit=501", wantErr: true},
		{name: "limit at max", query: "?limit=500", wantErr: false},
		{name: "empty sort field", query: "?sort=-", wantErr: true},
		{name: "sort with invalid chars", query: "?sort=name!", wantErr: true},
		{name: "sort with spaces", query: "?sort=my field", wantErr: true},
		{name: "malformed filter no colon", query: "?filter=type", wantErr: true},
		{name: "malformed filter empty key", query: "?filter=:value", wantErr: true},
		{name: "empty fields", query: "?fields=", wantErr: true},
		{name: "filter with empty value OK", query: "?filter=type:", wantErr: false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := ParseODataQuery(newTestRequest(tc.query))
			if tc.wantErr && err == nil {
				t.Errorf("expected error but got nil")
			}
			if !tc.wantErr && err != nil {
				t.Errorf("expected no error but got: %v", err)
			}
		})
	}
}

func TestParseODataQuery_FilterEdgeCases(t *testing.T) {
	tests := []struct {
		name       string
		query      string
		wantFilter map[string]string
		wantErr    bool
	}{
		{
			name:       "empty filter param",
			query:      "?filter=",
			wantFilter: nil,
			wantErr:    false,
		},
		{
			name:       "filter with empty value",
			query:      "?filter=type:",
			wantFilter: map[string]string{"type": ""},
			wantErr:    false,
		},
		{
			name:       "filter with multiple colons",
			query:      "?filter=url:http://example.com:8080",
			wantFilter: map[string]string{"url": "http://example.com:8080"},
			wantErr:    false,
		},
		{
			name:       "filter with comma in value is split",
			query:      "?filter=a:1,b:2",
			wantFilter: map[string]string{"a": "1", "b": "2"},
			wantErr:    false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			qry, err := ParseODataQuery(newTestRequest(tc.query))
			if tc.wantErr {
				if err == nil {
					t.Errorf("expected error but got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tc.wantFilter == nil {
				if qry.Filter != nil {
					t.Errorf("expected nil Filter, got %v", qry.Filter)
				}
				return
			}
			if len(qry.Filter) != len(tc.wantFilter) {
				t.Fatalf("Filter: want %v, got %v", tc.wantFilter, qry.Filter)
			}
			for k, v := range tc.wantFilter {
				if qry.Filter[k] != v {
					t.Errorf("Filter[%q]: want %q, got %q", k, v, qry.Filter[k])
				}
			}
		})
	}
}

func TestParseODataQuery_ErrorMessages(t *testing.T) {
	_, err := ParseODataQuery(newTestRequest("?page=-1&limit=999&sort=-"))
	if err == nil {
		t.Fatal("expected error for multiple invalid params")
	}
	msg := err.Error()
	if !strings.Contains(msg, "page must be a positive integer") {
		t.Errorf("error message missing page error: %s", msg)
	}
	if !strings.Contains(msg, "limit must not exceed 500") {
		t.Errorf("error message missing limit error: %s", msg)
	}
	if !strings.Contains(msg, "sort field must not be empty") {
		t.Errorf("error message missing sort error: %s", msg)
	}
}

func TestParseFilter(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    map[string]string
		wantErr bool
	}{
		{
			name:  "simple pair",
			input: "key:value",
			want:  map[string]string{"key": "value"},
		},
		{
			name:  "multiple pairs",
			input: "a:1,b:2,c:3",
			want:  map[string]string{"a": "1", "b": "2", "c": "3"},
		},
		{
			name:    "missing colon",
			input:   "badpair",
			wantErr: true,
		},
		{
			name:    "empty key",
			input:   ":value",
			wantErr: true,
		},
		{
			name:  "empty value OK",
			input: "key:",
			want:  map[string]string{"key": ""},
		},
		{
			name:  "trailing comma produces empty pair skipped",
			input: "a:1,",
			want:  map[string]string{"a": "1"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result, err := parseFilter(tc.input)
			if tc.wantErr {
				if err == nil {
					t.Error("expected error but got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(result) != len(tc.want) {
				t.Fatalf("want %v, got %v", tc.want, result)
			}
			for k, v := range tc.want {
				if result[k] != v {
					t.Errorf("[%q] want %q, got %q", k, v, result[k])
				}
			}
		})
	}
}

func TestParseFields(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  []string
	}{
		{name: "single", input: "name", want: []string{"name"}},
		{name: "multiple", input: "a,b,c", want: []string{"a", "b", "c"}},
		{name: "with spaces", input: " a , b , c ", want: []string{"a", "b", "c"}},
		{name: "empty string", input: "", want: []string{}},
		{name: "only commas", input: ",,,", want: []string{}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := parseFields(tc.input)
			if len(got) != len(tc.want) {
				t.Fatalf("want %v, got %v", tc.want, got)
			}
			for i, v := range tc.want {
				if got[i] != v {
					t.Errorf("[%d] want %q, got %q", i, v, got[i])
				}
			}
		})
	}
}

func TestSortDirectionConstants(t *testing.T) {
	if SortASC != "ASC" {
		t.Errorf("SortASC: want ASC, got %s", SortASC)
	}
	if SortDESC != "DESC" {
		t.Errorf("SortDESC: want DESC, got %s", SortDESC)
	}
}

func TestIsValidSortField(t *testing.T) {
	tests := []struct {
		input string
		want  bool
	}{
		{input: "name", want: true},
		{input: "sub_type", want: true},
		{input: "my-field", want: true},
		{input: "field123", want: true},
		{input: "CamelCase", want: true},
		{input: "name!", want: false},
		{input: "my field", want: false},
		{input: "a.b", want: false},
		{input: "", want: true}, // empty is technically valid at regex level; caller checks non-empty
	}

	for _, tc := range tests {
		t.Run(tc.input, func(t *testing.T) {
			got := isValidSortField(tc.input)
			if got != tc.want {
				t.Errorf("isValidSortField(%q) = %v, want %v", tc.input, got, tc.want)
			}
		})
	}
}

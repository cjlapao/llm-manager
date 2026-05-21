# OData Query Layer

The OData query layer adds structured filtering, pagination, sorting, search, and field projection to any list endpoint in the llm-manager API.

## Overview

The OData query layer lets you refine list responses without changing the API surface. When you send **no** OData query parameters, endpoints return the same flat JSON array they always have (backward compatible). When you send **any** OData parameter, the response switches to a wrapped format with pagination metadata.

This document covers the query parameters, response formats, concrete examples, error handling, and how to add OData support to new endpoints.

## Query Parameters

All parameters are optional. They are passed as standard URL query parameters.

| Parameter | Syntax | Default | Description |
|-----------|--------|---------|-------------|
| `page` | Integer, 1-based | `1` | Page number to return |
| `limit` | Integer, 1–500 | `25` | Number of results per page (maximum 500) |
| `sort` | `fieldname` or `-fieldname` | none | Sort ascending by field; prefix with `-` for descending |
| `filter` | `key:value,key:value` | none | Filter by exact field matches (comma-separated) |
| `search` | Free text string | none | Case-insensitive search against `name` and `slug` fields |
| `fields` | `field,field` | none | Return only specified fields (comma-separated, white-listed) |

### Parameter Details

#### `page` and `limit` — Pagination

`page` is 1-based (the first page is `1`). `limit` controls how many results appear per page. The API enforces a hard maximum of 500 results per page.

```
GET /api/models?page=2&limit=10
```

#### `sort` — Sorting

Prefix the field name with `-` to sort descending. Without a prefix, sorting is ascending. The field name must be in the white-list for the model type.

```
# Ascending by name
GET /api/models?sort=name

# Descending by creation date
GET /api/models?sort=-created_at
```

Allowed characters in sort field names: letters (`a–z`, `A–Z`), digits (`0–9`), underscores (`_`), and hyphens (`-`). Empty sort fields (e.g. `sort=-`) are rejected.

#### `filter` — Filtering

Filter by exact field matches. The syntax is comma-separated `key:value` pairs. Each pair matches records where the field equals the value exactly.

```
# Filter by type and status
GET /api/models?filter=type:llm,status:running

# Filter by a single field
GET /api/models?filter=engine_type:vllm
```

All filter field names must be in the white-list for the model type. Unknown fields produce a 400 error.

#### `search` — Text Search

Performs a case-insensitive `LIKE` search against the `name` and `slug` fields.

```
GET /api/models?search=gpt
```

#### `fields` — Field Projection

Return only the specified fields, reducing payload size. Field names must be in the white-list for the model type.

```
GET /api/models?fields=slug,name,type,port
```

## Response Format

The response shape depends on whether OData parameters are present.

### Wrapped Response (OData parameters present)

When any OData parameter is included, the response is wrapped in an envelope:

```json
{
  "data": [...],
  "meta": {
    "total": 42,
    "page": 1,
    "limit": 25,
    "total_pages": 2
  }
}
```

| Field | Type | Description |
|-------|------|-------------|
| `data` | Array | The filtered/paginated list of records |
| `meta.total` | Integer | Total number of records matching the query (before pagination) |
| `meta.page` | Integer | The current page number |
| `meta.limit` | Integer | The number of results per page |
| `meta.total_pages` | Integer | Total number of pages available |

`total_pages` is calculated as `ceil(total / limit)`.

### Flat Array (no OData parameters)

When **no** OData parameters are sent, the endpoint returns the same flat JSON array it always has. This preserves backward compatibility with existing clients.

```
GET /api/models
```

```json
[
  {
    "id": "550e8400-e29b-41d4-a716-446655440000",
    "slug": "gpt-4",
    "name": "GPT-4",
    "type": "llm",
    "port": 8000
  }
]
```

## Examples

### Pagination

Return page 2 of results, 10 per page:

```bash
curl 'http://localhost:8080/api/models?page=2&limit=10'
```

Response:

```json
{
  "data": [
    { "id": "...", "slug": "model-11", "name": "Model 11", ... },
    { "id": "...", "slug": "model-12", "name": "Model 12", ... }
  ],
  "meta": {
    "total": 23,
    "page": 2,
    "limit": 10,
    "total_pages": 3
  }
}
```

### Sorting

Sort models by name descending:

```bash
curl 'http://localhost:8080/api/models?sort=-name'
```

### Filtering

Filter models by type `llm` and engine type `vllm`:

```bash
curl 'http://localhost:8080/api/models?filter=type:llm,engine_type:vllm'
```

### Search

Search for models matching "gpt":

```bash
curl 'http://localhost:8080/api/models?search=gpt'
```

### Field Projection

Return only `slug`, `name`, and `port`:

```bash
curl 'http://localhost:8080/api/models?fields=slug,name,port'
```

### Combined Query

Combine all parameters in a single request:

```bash
curl 'http://localhost:8080/api/models?page=1&limit=10&sort=-created_at&filter=type:llm&search=gpt&fields=slug,name,type,created_at'
```

This returns the first page of 10 LLM models matching "gpt", sorted by creation date descending, with only the specified fields.

### RAG Endpoint

The `/api/rag` endpoint supports the same OData parameters. RAG models have a custom field white-list:

```
slug, name, type, sub_type, container, port, engine_type, created_at, updated_at, status
```

```bash
curl 'http://localhost:8080/api/rag?sort=-name&fields=slug,name,status'
```

## How to Add OData Support to a New Endpoint

The pattern is consistent across list endpoints. Follow these steps:

### Step 1: Parse OData parameters

Call `ParseODataQuery(r)` at the start of your handler:

```go
opts, err := ParseODataQuery(r)
if err != nil {
    WriteError(w, http.StatusBadRequest, "invalid query parameters: "+err.Error())
    return
}
```

### Step 2: Check if OData parameters are present

Compare the parsed options against their defaults:

```go
hasOData := opts.Page != 1 || opts.Limit != 25 || opts.SortField != "" ||
    len(opts.Filter) > 0 || opts.Search != "" || len(opts.Fields) > 0
```

### Step 3: Return flat array when no OData params

When `hasOData` is `false`, return the same flat array the endpoint always has:

```go
if !hasOData {
    items, err := h.ItemService.ListItems()
    if err != nil {
        WriteError(w, http.StatusInternalServerError, "failed to list items: "+err.Error())
        return
    }
    WriteJSON(w, http.StatusOK, items)
    return
}
```

### Step 4: Apply OData filters when params are present

When `hasOData` is `true`, build a GORM query, apply filters, and return a wrapped response:

```go
db := h.DB.DB()

filtered, total, err := ApplyODataFilters(db.Model(&models.Item{}), ODataOptions{
    ODataQuery: opts,
    ModelTable: "items",
    ModelType:  &models.Item{},
})
if err != nil {
    WriteError(w, http.StatusBadRequest, err.Error())
    return
}

var items []models.Item
if err := filtered.Find(&items).Error; err != nil {
    WriteError(w, http.StatusInternalServerError, "failed to query items: "+err.Error())
    return
}

limit := opts.Limit
if limit == 0 {
    limit = 25
}
totalPages := int((total + int64(limit) - 1) / int64(limit))

resp := ODataListResponse{
    Data: items,
    Meta: ODataMeta{
        Total:      int(total),
        Page:       opts.Page,
        Limit:      limit,
        TotalPages: totalPages,
    },
}

WriteJSON(w, http.StatusOK, resp)
```

### Step 5: Register the route

Ensure the route is registered in your router and the `JSONEnvelope` middleware is applied. The middleware automatically detects OData-enveloped responses (those with both `data` and `meta` keys) and passes them through unchanged, avoiding double-wrapping.

## Field White-List

The field white-list prevents clients from requesting arbitrary database columns. Only fields in the white-list for a given model type can be used with `sort`, `filter`, and `fields` parameters.

### Built-in Model Types

| Model Table | Allowed Fields |
|-------------|----------------|
| `models` | `id`, `slug`, `type`, `sub_type`, `name`, `hf_repo`, `yml`, `container`, `port`, `engine_type`, `env_vars`, `command_args`, `input_token_cost`, `output_token_cost`, `capabilities`, `lite_llm_params`, `model_info`, `litellm_model_id`, `litellm_active_aliases`, `litellm_variant_ids`, `default`, `base_image_id`, `engine_version_slug`, `total_params_b`, `active_params_b`, `is_moe`, `attention_layers`, `gdn_layers`, `num_kv_heads`, `head_dim`, `supports_mtp`, `default_context`, `max_context`, `quant_bytes_per_param`, `max_num_seqs`, `max_num_batched_tokens`, `speculative_decoding`, `num_speculative_tokens`, `created_at`, `updated_at` |
| `containers` | `id`, `slug`, `name`, `status`, `port`, `gpu_used`, `created_at`, `updated_at` |
| `base_images` | `id`, `slug`, `name`, `engine_type`, `docker_image`, `entrypoint`, `environment_json`, `volumes_json`, `composed_yml_file`, `created_at`, `updated_at` |
| `hotspots` | `id`, `model_slug`, `active`, `created_at`, `updated_at` |
| `config` | `id`, `key`, `value`, `created_at`, `updated_at` |
| `engine_types` | `id`, `slug`, `name`, `description`, `created_at`, `updated_at` |
| `engine_versions` | `id`, `slug`, `engine_type_slug`, `version`, `is_default`, `is_latest`, `created_at`, `updated_at` |

### How to Add a New Model Type to the White-List

Edit `internal/api/odata_fields.go` and add a new entry to the `FieldWhiteLists` map:

```go
var FieldWhiteLists = map[string]FieldWhiteList{
    // ... existing entries ...

    // Your new model type
    "my_models": {
        Columns: []string{
            "id", "slug", "name", "status", "created_at", "updated_at",
        },
    },
}
```

The key is the database table name (as used in GORM). The `Columns` slice lists every field that clients may request via `sort`, `filter`, and `fields`.

### Custom White-Lists for Specialized Endpoints

Some endpoints (like `/api/rag`) need a custom white-list that extends or differs from the base table. In such cases, define a custom slice and validation function:

```go
var ragModelWhiteList = []string{
    "slug", "name", "type", "sub_type", "container", "port",
    "engine_type", "created_at", "updated_at", "status",
}

func validateRagFields(fields []string) error {
    // Check each field against ragModelWhiteList
    // Return error listing unknown fields
}
```

The RAG endpoint uses this pattern because it computes a `status` field from container state, which is not a direct database column.

## Error Handling

### Validation Errors (400 Bad Request)

The API returns `400` when a query parameter fails validation:

| Condition | Example Error |
|-----------|---------------|
| `page` is not a positive integer | `invalid query parameters: page must be a positive integer` |
| `limit` is not a positive integer | `invalid query parameters: limit must be a positive integer` |
| `limit` exceeds 500 | `invalid query parameters: limit must not exceed 500` |
| `sort` field is empty | `invalid query parameters: sort field must not be empty` |
| `sort` field contains invalid characters | `invalid query parameters: sort field "123!" contains invalid characters (use letters, digits, underscore, hyphen)` |
| `filter` pair is malformed (no colon or empty key) | `invalid query parameters: malformed filter pair "" (expected key:value)` |
| `fields` is empty or has no valid names | `invalid query parameters: fields parameter must contain at least one field name` |
| `fields` contains unknown fields | `invalid query parameters: unsupported field(s): bad_field (allowed: id, slug, name, ...)` |
| `sort` or `filter` field not in white-list | `sort field "bad_field": no field white-list defined for model table "..."` or `unsupported field(s): ...` |

Error response format:

```json
{
  "success": false,
  "data": null,
  "error": "invalid query parameters: page must be a positive integer",
  "status": 400
}
```

### Server Errors (500 Internal Server Error)

The API returns `500` when an internal failure occurs:

| Condition | Example Error |
|-----------|---------------|
| Database query fails | `failed to query models: <database error>` |
| Count query fails | `failed to count records: <database error>` |

Error response format:

```json
{
  "success": false,
  "data": null,
  "error": "failed to query models: sql: database is closed",
  "status": 500
}
```

### JSON Envelope Wrapping

All responses pass through the `JSONEnvelope` middleware, which wraps them in:

```json
{
  "success": true/false,
  "data": <response body>,
  "error": <error message if failed>,
  "status": <HTTP status code>
}
```

OData-enveloped responses (those with both `data` and `meta` top-level keys) are detected and passed through unchanged to avoid double-wrapping.

Non-JSON responses (YAML, plain text) and `204 No Content` responses bypass the envelope entirely.

## Limitations

The following are **not** supported by the current OData query layer:

| Feature | Status | Reason |
|---------|--------|--------|
| Nested field filtering (e.g. `filter=metadata.key:value`) | Not supported | Filters match top-level columns only |
| Range operators (e.g. `filter=age>18`) | Not supported | Only exact equality (`=`) via `key:value` syntax |
| Logical operators (`and`, `or`, `not`) | Not supported | Filters are combined with implicit `AND` |
| Field aliases or computed expressions | Not supported | Fields must map to actual database columns |
| `skip` / `top` syntax | Not supported | Uses `page` and `limit` only |
| `$count` query | Not supported | Total is always included in `meta` |
| Case-insensitive filter values | Not supported | Filter values are matched exactly |
| Wildcard search in `search` parameter | Not supported | The search term is wrapped with `%` on both sides internally |

## See Also

- [Architecture](./architecture.md) — System architecture overview
- [API Endpoints](../README.md) — API endpoint reference in the README

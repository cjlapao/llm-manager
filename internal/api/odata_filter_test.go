package api

import (
	"os"
	"testing"

	"github.com/user/llm-manager/internal/database/models"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func newTestDB(t *testing.T) (*gorm.DB, func()) {
	t.Helper()
	f, err := os.CreateTemp("", "odata-test-*.db")
	if err != nil {
		t.Fatalf("failed to create temp db: %v", err)
	}
	dbPath := f.Name()

	db, err := gorm.Open(sqlite.Open(dbPath), &gorm.Config{})
	if err != nil {
		f.Close()
		t.Fatalf("failed to open db: %v", err)
	}

	if err := db.AutoMigrate(&models.Model{}); err != nil {
		f.Close()
		t.Fatalf("failed to migrate: %v", err)
	}

	cleanup := func() {
		sqlDB, _ := db.DB()
		if sqlDB != nil {
			sqlDB.Close()
		}
		os.Remove(dbPath)
		f.Close()
	}

	return db, cleanup
}

func seedModels(t *testing.T, db *gorm.DB) {
	t.Helper()
	models := []models.Model{
		{Slug: "llama-3-8b", Name: "Llama 3 8B", Type: "llm", SubType: "chat", Port: 8001},
		{Slug: "llama-3-70b", Name: "Llama 3 70B", Type: "llm", SubType: "chat", Port: 8002},
		{Slug: "mistral-7b", Name: "Mistral 7B", Type: "llm", SubType: "chat", Port: 8003},
		{Slug: "dbrx-instruct", Name: "DBRX Instruct", Type: "llm", SubType: "chat", Port: 8004},
		{Slug: "phi-3-mini", Name: "Phi-3 Mini", Type: "llm", SubType: "chat", Port: 8005},
		{Slug: "gemma-2-9b", Name: "Gemma 2 9B", Type: "llm", SubType: "instruct", Port: 8006},
		{Slug: "qwen-2-7b", Name: "Qwen 2 7B", Type: "llm", SubType: "chat", Port: 8007},
		{Slug: "olmo-7b", Name: "OLMo 7B", Type: "llm", SubType: "chat", Port: 8008},
		{Slug: "yi-34b", Name: "Yi 34B", Type: "llm", SubType: "chat", Port: 8009},
		{Slug: "falcon-180b", Name: "Falcon 180B", Type: "llm", SubType: "chat", Port: 8010},
	}
	if err := db.Create(&models).Error; err != nil {
		t.Fatalf("failed to seed models: %v", err)
	}
}

func TestApplyODataFilters_Pagination(t *testing.T) {
	db, cleanup := newTestDB(t)
	defer cleanup()
	seedModels(t, db)

	opts := ODataOptions{
		ODataQuery: ODataQuery{
			Page:  2,
			Limit: 3,
		},
		ModelTable: "models",
		ModelType:  &models.Model{},
	}

	result, total, err := ApplyODataFilters(db.Model(&models.Model{}), opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if total != 10 {
		t.Errorf("expected total=10, got %d", total)
	}

	var modelsList []models.Model
	if err := result.Find(&modelsList).Error; err != nil {
		t.Fatalf("query failed: %v", err)
	}
	if len(modelsList) != 3 {
		t.Errorf("expected 3 results, got %d", len(modelsList))
	}
}

func TestApplyODataFilters_Sorting(t *testing.T) {
	db, cleanup := newTestDB(t)
	defer cleanup()
	seedModels(t, db)

	opts := ODataOptions{
		ODataQuery: ODataQuery{
			SortField: "slug",
			SortDir:   SortASC,
		},
		ModelTable: "models",
		ModelType:  &models.Model{},
	}

	result, _, err := ApplyODataFilters(db.Model(&models.Model{}), opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var modelsList []models.Model
	if err := result.Find(&modelsList).Error; err != nil {
		t.Fatalf("query failed: %v", err)
	}
	if len(modelsList) == 0 {
		t.Fatal("expected results")
	}
	firstSlug := modelsList[0].Slug
	lastSlug := modelsList[len(modelsList)-1].Slug
	if firstSlug > lastSlug {
		t.Errorf("expected ascending order, got %s > %s", firstSlug, lastSlug)
	}
}

func TestApplyODataFilters_SortingDESC(t *testing.T) {
	db, cleanup := newTestDB(t)
	defer cleanup()
	seedModels(t, db)

	opts := ODataOptions{
		ODataQuery: ODataQuery{
			SortField: "slug",
			SortDir:   SortDESC,
		},
		ModelTable: "models",
		ModelType:  &models.Model{},
	}

	result, _, err := ApplyODataFilters(db.Model(&models.Model{}), opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var modelsList []models.Model
	if err := result.Find(&modelsList).Error; err != nil {
		t.Fatalf("query failed: %v", err)
	}
	if len(modelsList) == 0 {
		t.Fatal("expected results")
	}
	firstSlug := modelsList[0].Slug
	lastSlug := modelsList[len(modelsList)-1].Slug
	if firstSlug < lastSlug {
		t.Errorf("expected descending order, got %s < %s", firstSlug, lastSlug)
	}
}

func TestApplyODataFilters_Filtering(t *testing.T) {
	db, cleanup := newTestDB(t)
	defer cleanup()
	seedModels(t, db)

	opts := ODataOptions{
		ODataQuery: ODataQuery{
			Filter: map[string]string{"type": "llm"},
		},
		ModelTable: "models",
		ModelType:  &models.Model{},
	}

	result, _, err := ApplyODataFilters(db.Model(&models.Model{}), opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var modelsList []models.Model
	if err := result.Find(&modelsList).Error; err != nil {
		t.Fatalf("query failed: %v", err)
	}
	if len(modelsList) != 10 {
		t.Errorf("expected 10 results, got %d", len(modelsList))
	}
}

func TestApplyODataFilters_FilteringSubType(t *testing.T) {
	db, cleanup := newTestDB(t)
	defer cleanup()
	seedModels(t, db)

	opts := ODataOptions{
		ODataQuery: ODataQuery{
			Filter: map[string]string{"sub_type": "instruct"},
		},
		ModelTable: "models",
		ModelType:  &models.Model{},
	}

	result, _, err := ApplyODataFilters(db.Model(&models.Model{}), opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var modelsList []models.Model
	if err := result.Find(&modelsList).Error; err != nil {
		t.Fatalf("query failed: %v", err)
	}
	if len(modelsList) != 1 {
		t.Errorf("expected 1 result, got %d", len(modelsList))
	}
	if modelsList[0].Slug != "gemma-2-9b" {
		t.Errorf("expected gemma-2-9b, got %s", modelsList[0].Slug)
	}
}

func TestApplyODataFilters_Search(t *testing.T) {
	db, cleanup := newTestDB(t)
	defer cleanup()
	seedModels(t, db)

	opts := ODataOptions{
		ODataQuery: ODataQuery{
			Search: "Llama",
		},
		ModelTable: "models",
		ModelType:  &models.Model{},
	}

	result, _, err := ApplyODataFilters(db.Model(&models.Model{}), opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var modelsList []models.Model
	if err := result.Find(&modelsList).Error; err != nil {
		t.Fatalf("query failed: %v", err)
	}
	if len(modelsList) != 2 {
		t.Errorf("expected 2 results for 'Llama', got %d", len(modelsList))
	}
}

func TestApplyODataFilters_SearchCaseInsensitive(t *testing.T) {
	db, cleanup := newTestDB(t)
	defer cleanup()
	seedModels(t, db)

	opts := ODataOptions{
		ODataQuery: ODataQuery{
			Search: "llama",
		},
		ModelTable: "models",
		ModelType:  &models.Model{},
	}

	result, _, err := ApplyODataFilters(db.Model(&models.Model{}), opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var modelsList []models.Model
	if err := result.Find(&modelsList).Error; err != nil {
		t.Fatalf("query failed: %v", err)
	}
	if len(modelsList) != 2 {
		t.Errorf("expected 2 results for 'llama' (case-insensitive), got %d", len(modelsList))
	}
}

func TestApplyODataFilters_FieldProjection(t *testing.T) {
	db, cleanup := newTestDB(t)
	defer cleanup()
	seedModels(t, db)

	opts := ODataOptions{
		ODataQuery: ODataQuery{
			Fields: []string{"slug", "name"},
		},
		ModelTable: "models",
		ModelType:  &models.Model{},
	}

	result, _, err := ApplyODataFilters(db.Model(&models.Model{}), opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var modelsList []models.Model
	if err := result.Find(&modelsList).Error; err != nil {
		t.Fatalf("query failed: %v", err)
	}
	if len(modelsList) != 10 {
		t.Errorf("expected 10 results, got %d", len(modelsList))
	}
	// Verify only slug and name are populated (other fields should be zero/empty)
	for _, m := range modelsList {
		if m.Slug == "" || m.Name == "" {
			t.Error("expected slug and name to be populated")
		}
		if m.Port != 0 {
			t.Error("expected port to be empty (not selected)")
		}
	}
}

func TestApplyODataFilters_TotalCount(t *testing.T) {
	db, cleanup := newTestDB(t)
	defer cleanup()
	seedModels(t, db)

	opts := ODataOptions{
		ODataQuery: ODataQuery{
			Page:   1,
			Limit:  3,
			Filter: map[string]string{"type": "llm"},
		},
		ModelTable: "models",
		ModelType:  &models.Model{},
	}

	_, total, err := ApplyODataFilters(db.Model(&models.Model{}), opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if total != 10 {
		t.Errorf("expected total=10, got %d", total)
	}
}

func TestApplyODataFilters_InvalidSortField(t *testing.T) {
	db, cleanup := newTestDB(t)
	defer cleanup()
	seedModels(t, db)

	opts := ODataOptions{
		ODataQuery: ODataQuery{
			SortField: "nonexistent_column",
		},
		ModelTable: "models",
		ModelType:  &models.Model{},
	}

	_, _, err := ApplyODataFilters(db.Model(&models.Model{}), opts)
	if err == nil {
		t.Fatal("expected error for invalid sort field")
	}
}

func TestApplyODataFilters_InvalidFilterField(t *testing.T) {
	db, cleanup := newTestDB(t)
	defer cleanup()
	seedModels(t, db)

	opts := ODataOptions{
		ODataQuery: ODataQuery{
			Filter: map[string]string{"nonexistent": "value"},
		},
		ModelTable: "models",
		ModelType:  &models.Model{},
	}

	_, _, err := ApplyODataFilters(db.Model(&models.Model{}), opts)
	if err == nil {
		t.Fatal("expected error for invalid filter field")
	}
}

func TestApplyODataFilters_InvalidFields(t *testing.T) {
	db, cleanup := newTestDB(t)
	defer cleanup()
	seedModels(t, db)

	opts := ODataOptions{
		ODataQuery: ODataQuery{
			Fields: []string{"slug", "nonexistent_field"},
		},
		ModelTable: "models",
		ModelType:  &models.Model{},
	}

	_, _, err := ApplyODataFilters(db.Model(&models.Model{}), opts)
	if err == nil {
		t.Fatal("expected error for invalid field")
	}
}

func TestApplyODataFilters_NoFilters(t *testing.T) {
	db, cleanup := newTestDB(t)
	defer cleanup()
	seedModels(t, db)

	opts := ODataOptions{
		ODataQuery: ODataQuery{},
		ModelTable: "models",
		ModelType:  &models.Model{},
	}

	result, total, err := ApplyODataFilters(db.Model(&models.Model{}), opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if total != 10 {
		t.Errorf("expected total=10, got %d", total)
	}

	var modelsList []models.Model
	if err := result.Find(&modelsList).Error; err != nil {
		t.Fatalf("query failed: %v", err)
	}
	if len(modelsList) != 10 {
		t.Errorf("expected all 10 results, got %d", len(modelsList))
	}
}

func TestApplyODataFilters_Combined(t *testing.T) {
	db, cleanup := newTestDB(t)
	defer cleanup()
	seedModels(t, db)

	opts := ODataOptions{
		ODataQuery: ODataQuery{
			Page:      1,
			Limit:     3,
			SortField: "slug",
			SortDir:   SortASC,
			Filter:    map[string]string{"sub_type": "chat"},
			Search:    "llama",
		},
		ModelTable: "models",
		ModelType:  &models.Model{},
	}

	result, total, err := ApplyODataFilters(db.Model(&models.Model{}), opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if total != 2 {
		t.Errorf("expected total=2, got %d", total)
	}

	var modelsList []models.Model
	if err := result.Find(&modelsList).Error; err != nil {
		t.Fatalf("query failed: %v", err)
	}
	if len(modelsList) != 2 {
		t.Errorf("expected 2 results, got %d", len(modelsList))
	}
}

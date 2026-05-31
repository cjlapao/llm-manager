// Package api provides the HTTP API server for llm-manager.
//
// swagger:meta
//
// @title llm-manager API
// @version 1.0
// @description A CLI tool and API server for managing LLM resources, containers, and RAG pipelines.
// @host localhost:8080
// @BasePath /api
//
//go:generate swag init --parseDependency --parseInternal --parseDepth 3 -g server.go -o ../../docs
package api

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gorilla/mux"
)

const defaultShutdownTimeout = 15 * time.Second

// StartAPIServer starts the HTTP API server on the given host and port.
// It registers the /api/ routes, sets up middleware, and handles graceful shutdown.
// The server runs in a blocking call until shutdown is triggered.
func StartAPIServer(ctx *APIContext, host string, port int, shutdownTimeout time.Duration) error {
	if shutdownTimeout == 0 {
		shutdownTimeout = defaultShutdownTimeout
	}

	router := mux.NewRouter()

	// Apply JSON envelope middleware to all /api/ routes
	api := router.PathPrefix("/api").Subrouter()
	api.Use(JSONEnvelope)

	// Register routes — handlers registered by task branches
	h := &APIContext{DB: ctx.DB, Config: ctx.Config, ModelService: ctx.ModelService, ContainerService: ctx.ContainerService}

	// Model routes (from model_handler.go)
	modelHandler := &ModelHandler{h}
	api.HandleFunc("/models", modelHandler.ListModels).Methods(http.MethodGet)
	api.HandleFunc("/models", modelHandler.CreateModel).Methods(http.MethodPost)
	api.HandleFunc("/models/{slug}", modelHandler.GetModel).Methods(http.MethodGet)
	api.HandleFunc("/models/{slug}", modelHandler.UpdateModel).Methods(http.MethodPut)
	api.HandleFunc("/models/{slug}", modelHandler.DeleteModel).Methods(http.MethodDelete)
	api.HandleFunc("/models/{slug}/info", modelHandler.GetModelInfo).Methods(http.MethodGet)

	// Model utility routes (from model_util_handler.go)
	modelUtilHandler := &ModelUtilHandler{h}
	api.HandleFunc("/models/import", modelUtilHandler.ImportModel).Methods(http.MethodPost)
	api.HandleFunc("/models/{slug}/export", modelUtilHandler.ExportModel).Methods(http.MethodGet)
	api.HandleFunc("/models/{slug}/compose", modelUtilHandler.ComposeModel).Methods(http.MethodGet)
	api.HandleFunc("/models/{slug}/cache", modelUtilHandler.ClearCache).Methods(http.MethodDelete)

	// RAG routes (from rag_handler.go)
	ragHandler := &RAGHandler{h}
	api.HandleFunc("/rag", ragHandler.ListRAG).Methods(http.MethodGet)
	api.HandleFunc("/rag/start", ragHandler.StartRAG).Methods(http.MethodPost)
	api.HandleFunc("/rag/stop", ragHandler.StopRAG).Methods(http.MethodPost)

	// Placeholder health check to verify server is running
	api.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		WriteJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	}).Methods(http.MethodGet)

	addr := fmt.Sprintf("%s:%d", host, port)

	srv := &http.Server{
		Addr:              addr,
		Handler:           router,
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	// Channel to listen for shutdown signals
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)

	fmt.Printf("llm-manager API server starting on %s\n", addr)

	// Start server in a goroutine
	go func() {
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			fmt.Fprintf(os.Stderr, "API server error: %v\n", err)
			os.Exit(1)
		}
	}()

	// Block until shutdown signal
	<-stop

	fmt.Println("\nShutting down API server...")

	// Graceful shutdown with timeout
	shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		fmt.Fprintf(os.Stderr, "API server forced shutdown: %v\n", err)
		return err
	}

	fmt.Println("API server stopped")
	return nil
}

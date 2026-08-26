package api

import (
	"net/http"
	"os"
	"path/filepath"
)

func (s *Server) registerRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/services", s.handleListServices)
	mux.HandleFunc("GET /api/services/{name}", s.handleGetService)
	mux.HandleFunc("POST /api/services", s.handleAddService)
	mux.HandleFunc("DELETE /api/services/{name}", s.handleDeleteService)
	mux.HandleFunc("POST /api/services/{name}/start", s.handleStartService)
	mux.HandleFunc("POST /api/services/{name}/stop", s.handleStopService)
	mux.HandleFunc("POST /api/services/{name}/restart", s.handleRestartService)
	mux.HandleFunc("GET /api/services/{name}/logs", s.handleGetLogs)
	mux.HandleFunc("GET /api/services/{name}/logs/stream", s.handleSSELogs)
	mux.HandleFunc("GET /api/manager/logs", s.handleManagerLogs)
	mux.HandleFunc("GET /api/manager/logs/stream", s.handleSSEManagerLogs)
	mux.HandleFunc("GET /api/config", s.handleGetConfig)
	mux.HandleFunc("PUT /api/config", s.handleUpdateConfig)
	mux.HandleFunc("PUT /api/config/port", s.handleUpdatePort)
	mux.HandleFunc("GET /api/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":"ok"}`))
	})

	webDir := "web/dist"
	if exe, err := os.Executable(); err == nil {
		absWebDir := filepath.Join(filepath.Dir(exe), "web", "dist")
		if _, err := os.Stat(absWebDir); err == nil {
			webDir = absWebDir
		}
	}
	mux.Handle("/", http.FileServer(http.Dir(webDir)))
}

package api

import "net/http"

func (s *Server) registerRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/services", s.handleListServices)
	mux.HandleFunc("GET /api/services/{name}", s.handleGetService)
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
	mux.Handle("/", http.FileServer(http.Dir("web/dist")))
}

package api

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"time"

	"service-manager/internal/config"
)

// handleListServices returns all managed services and their status.
func (s *Server) handleListServices(w http.ResponseWriter, r *http.Request) {
	statuses := s.mgr.AllServiceStatus()
	writeJSON(w, http.StatusOK, map[string]any{
		"services": statuses,
	})
}

// handleGetService returns details for a single service.
func (s *Server) handleGetService(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	info, ok := s.mgr.ServiceInfo(name)
	if !ok {
		writeError(w, http.StatusNotFound, "service not found")
		return
	}
	writeJSON(w, http.StatusOK, info)
}

// handleStartService starts a named service.
func (s *Server) handleStartService(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if err := s.mgr.Supervisor().Start(name); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "started"})
}

// handleStopService stops a named service.
func (s *Server) handleStopService(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if err := s.mgr.Supervisor().Stop(name); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "stopped"})
}

// handleRestartService restarts a named service.
func (s *Server) handleRestartService(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if err := s.mgr.Supervisor().Restart(name); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "restarted"})
}

// handleGetLogs returns the last N lines of a service log.
func (s *Server) handleGetLogs(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	linesStr := r.URL.Query().Get("lines")
	lines := 100
	if linesStr != "" {
		if v, err := strconv.Atoi(linesStr); err == nil && v > 0 {
			lines = v
		}
	}
	mp, ok := s.mgr.Supervisor().Get(name)
	if !ok {
		writeError(w, http.StatusNotFound, "service not found")
		return
	}
	logLines, err := mp.LogTail(lines)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"service": name,
		"lines":   logLines,
	})
}

// handleSSELogs streams log events for a named service using SSE.
func (s *Server) handleSSELogs(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	mp, ok := s.mgr.Supervisor().Get(name)
	if !ok {
		writeError(w, http.StatusNotFound, "service not found")
		return
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, http.StatusInternalServerError, "streaming not supported")
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	// Tail the log file and stream new lines
	logPath := mp.LogFilePath()
	reader, cleanup := tailAndFollow(logPath)
	defer cleanup()
	defer cleanup()

	buf := make([]byte, 4096)
	for {
		select {
		case <-r.Context().Done():
			return
		default:
			n, err := reader.Read(buf)
			if n > 0 {
				line := string(buf[:n])
				fmt.Fprintf(w, "data: %s\n\n", line)
				flusher.Flush()
			}
			if err != nil {
				time.Sleep(500 * time.Millisecond)
			}
		}
	}
}

func (s *Server) handleGetConfig(w http.ResponseWriter, r *http.Request) {
	cfg := s.mgr.Loader().Get()
	fmt.Printf("[DEBUG] handleGetConfig: cfg=%v, services=%d\n", cfg != nil, len(cfg.Services))
	if cfg == nil {
		writeError(w, http.StatusInternalServerError, "no config loaded")
		return
	}
	for i, svc := range cfg.Services {
		fmt.Printf("[DEBUG]   service[%d]: name=%s exe=%s\n", i, svc.Name, svc.Executable)
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, cfg)
}

// handleUpdateConfig updates the config and reloads services.
func (s *Server) handleUpdateConfig(w http.ResponseWriter, r *http.Request) {
	var cfg config.ManagerConfig
	if err := json.NewDecoder(r.Body).Decode(&cfg); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}
	if err := s.mgr.Loader().UpdateAndSave(&cfg); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if err := s.mgr.Reload(); err != nil {
		writeError(w, http.StatusInternalServerError, "reload: "+err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "updated"})
}

// handleAddService adds a new service to the configuration.
func (s *Server) handleAddService(w http.ResponseWriter, r *http.Request) {
	var svc config.ServiceConfig
	if err := json.NewDecoder(r.Body).Decode(&svc); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}

	cfg := s.mgr.Loader().Get()
	if cfg == nil {
		writeError(w, http.StatusInternalServerError, "no config loaded")
		return
	}

	// Check for duplicate name
	for _, existing := range cfg.Services {
		if existing.Name == svc.Name {
			writeError(w, http.StatusConflict, "service with this name already exists")
			return
		}
	}

	cfg.Services = append(cfg.Services, svc)

	if err := s.mgr.Loader().UpdateAndSave(cfg); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if err := s.mgr.Reload(); err != nil {
		writeError(w, http.StatusInternalServerError, "reload: "+err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "added"})
}

// handleDeleteService removes a service from the configuration.
func (s *Server) handleDeleteService(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")

	cfg := s.mgr.Loader().Get()
	if cfg == nil {
		writeError(w, http.StatusInternalServerError, "no config loaded")
		return
	}

	found := false
	var newServices []config.ServiceConfig
	for _, svc := range cfg.Services {
		if svc.Name == name {
			found = true
			continue
		}
		newServices = append(newServices, svc)
	}

	if !found {
		writeError(w, http.StatusNotFound, "service not found")
		return
	}

	cfg.Services = newServices

	if err := s.mgr.Loader().UpdateAndSave(cfg); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if err := s.mgr.Reload(); err != nil {
		writeError(w, http.StatusInternalServerError, "reload: "+err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

// handleUpdatePort updates the web port and saves config.
func (s *Server) handleUpdatePort(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Port int `json:"port"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	cfg := s.mgr.Loader().Get()
	if cfg == nil {
		writeError(w, http.StatusInternalServerError, "no config")
		return
	}
	cfg.WebPort = body.Port
	if err := s.mgr.Loader().UpdateAndSave(cfg); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "updated", "note": "restart required to apply new port"})
}

// handleManagerLogs returns manager-level logs.
func (s *Server) handleManagerLogs(w http.ResponseWriter, r *http.Request) {
	linesStr := r.URL.Query().Get("lines")
	lines := 100
	if linesStr != "" {
		if v, err := strconv.Atoi(linesStr); err == nil && v > 0 {
			lines = v
		}
	}
	logLines, err := tailFile(s.mgr.Logger().LogPath(), lines)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"service": "manager",
		"lines":   logLines,
	})
}

// handleSSEManagerLogs streams manager logs via SSE.
func (s *Server) handleSSEManagerLogs(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, http.StatusInternalServerError, "streaming not supported")
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	logPath := s.mgr.Logger().LogPath()
	reader, cleanup := tailAndFollow(logPath)
	defer cleanup()

	buf := make([]byte, 4096)
	for {
		select {
		case <-r.Context().Done():
			return
		default:
			n, err := reader.Read(buf)
			if n > 0 {
				line := string(buf[:n])
				fmt.Fprintf(w, "data: %s\n\n", line)
				flusher.Flush()
			}
			if err != nil {
				time.Sleep(500 * time.Millisecond)
			}
		}
	}
}

// tailAndFollow returns an io.Reader that yields new content from a file,
// starting from the current end.
func tailAndFollow(path string) (io.Reader, func()) {
	pr, pw := io.Pipe()
	// Seek to end of file, then read new content
	f, err := openFileAtEnd(path)
	if err != nil {
		// File doesn't exist yet, just return pipe
		return pr, func() { pr.Close() }
	}
	go func() {
		defer f.Close()
		buf := make([]byte, 4096)
		for {
			n, err := f.Read(buf)
			if n > 0 {
				pw.Write(buf[:n])
			}
			if err != nil {
				time.Sleep(300 * time.Millisecond)
			}
		}
	}()
	return pr, func() { pr.Close() }
}

func openFileAtEnd(path string) (*fileReader, error) {
	f, err := openFile(path)
	if err != nil {
		return nil, err
	}
	// Seek to end
	f.Seek(0, io.SeekEnd)
	return f, nil
}

func tailFile(path string, n int) ([]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var lines []string
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	return lines, scanner.Err()
}

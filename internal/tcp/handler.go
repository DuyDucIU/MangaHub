package tcp

import (
	"encoding/json"
	"net/http"
)

// InternalHandler returns an HTTP handler for the internal broadcast endpoint.
// Only POST /internal/broadcast is accepted.
func (s *ProgressSyncServer) InternalHandler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/internal/broadcast", s.handleBroadcast)
	return mux
}

func (s *ProgressSyncServer) handleBroadcast(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var update ProgressUpdate
	if err := json.NewDecoder(r.Body).Decode(&update); err != nil {
		http.Error(w, "invalid payload", http.StatusBadRequest)
		return
	}
	select {
	case s.Broadcast <- update:
		w.WriteHeader(http.StatusOK)
	default:
		http.Error(w, "broadcast queue full", http.StatusServiceUnavailable)
	}
}

package udp

import (
	"encoding/json"
	"net/http"
)

// InternalHandler returns an HTTP handler for the internal notify endpoint.
// Only POST /internal/notify is accepted.
func (s *NotificationServer) InternalHandler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/internal/notify", s.handleNotify)
	return mux
}

func (s *NotificationServer) handleNotify(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req NotifyRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid payload", http.StatusBadRequest)
		return
	}
	select {
	case <-s.done:
		http.Error(w, "service unavailable", http.StatusServiceUnavailable)
	default:
		select {
		case s.Notify <- req:
			w.WriteHeader(http.StatusOK)
		default:
			http.Error(w, "notify queue full", http.StatusServiceUnavailable)
		}
	}
}

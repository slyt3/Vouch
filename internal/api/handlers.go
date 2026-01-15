package api

import (
	"fmt"
	"log"
	"net/http"
	"strings"

	"github.com/yourname/vouch/internal/core"
)

type Handlers struct {
	Core *core.Engine
}

func NewHandlers(engine *core.Engine) *Handlers {
	return &Handlers{Core: engine}
}

func (h *Handlers) HandleApprove(w http.ResponseWriter, r *http.Request) {
	eventID := strings.TrimPrefix(r.URL.Path, "/api/approve/")
	if eventID == "" {
		http.Error(w, "Event ID required", http.StatusBadRequest)
		return
	}

	val, ok := h.Core.StallSignals.Load(eventID)
	if !ok {
		http.Error(w, "Event not found", http.StatusNotFound)
		return
	}

	approvalChan := val.(chan bool)
	select {
	case approvalChan <- true:
		log.Printf("Event %s approved via API", eventID)
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("Approved\n"))
	default:
		http.Error(w, "Already processed", http.StatusConflict)
	}
}

func (h *Handlers) HandleReject(w http.ResponseWriter, r *http.Request) {
	eventID := strings.TrimPrefix(r.URL.Path, "/api/reject/")
	if eventID == "" {
		http.Error(w, "Event ID required", http.StatusBadRequest)
		return
	}

	val, ok := h.Core.StallSignals.Load(eventID)
	if !ok {
		http.Error(w, "Event not found", http.StatusNotFound)
		return
	}

	approvalChan := val.(chan bool)
	select {
	case approvalChan <- false:
		log.Printf("Event %s rejected via API", eventID)
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("Rejected\n"))
	default:
		http.Error(w, "Already processed", http.StatusConflict)
	}
}

func (h *Handlers) HandleRekey(w http.ResponseWriter, r *http.Request) {
	oldPubKey, newPubKey, err := h.Core.Worker.GetSigner().RotateKey(".vouch_key")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	log.Printf("Key rotated: %s -> %s", oldPubKey[:16], newPubKey[:16])
	fmt.Fprintf(w, "Key rotated\nOld: %s\nNew: %s", oldPubKey, newPubKey)
}

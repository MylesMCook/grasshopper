package gomcp

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/MylesMCook/grasshopper/internal/gomemory"
)

const pairingLifetime = 5 * time.Minute
const maxPendingPairings = 16

type pairingRequest struct {
	ID      string
	Code    string
	Device  string
	Hash    [sha256.Size]byte
	Expires time.Time
	Status  string
}

type pairingManager struct {
	mu       sync.Mutex
	requests map[string]*pairingRequest
	store    *gomemory.Writer
}

func newPairingManager(store *gomemory.Writer) *pairingManager {
	return &pairingManager{requests: make(map[string]*pairingRequest), store: store}
}

func pairingJSON(w http.ResponseWriter, value any, status int) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func readPairingJSON(w http.ResponseWriter, r *http.Request, value any) error {
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 512))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(value); err != nil {
		return err
	}
	if err := decoder.Decode(new(any)); !errors.Is(err, io.EOF) {
		return errors.New("trailing input")
	}
	return nil
}

func (p *pairingManager) expire(now time.Time) {
	for id, request := range p.requests {
		if !now.Before(request.Expires) {
			delete(p.requests, id)
		}
	}
}

func (p *pairingManager) start(w http.ResponseWriter, r *http.Request, origin string) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if header := r.Header.Get("Origin"); header != "" && header != origin {
		http.Error(w, "invalid Origin header", http.StatusForbidden)
		return
	}
	var input struct {
		Device    string `json:"device"`
		TokenHash string `json:"token_hash"`
	}
	if readPairingJSON(w, r, &input) != nil || strings.TrimSpace(input.Device) == "" || len(input.Device) > 64 || strings.IndexFunc(input.Device, unicode.IsControl) >= 0 {
		http.Error(w, "invalid pairing request", http.StatusBadRequest)
		return
	}
	hash, err := hex.DecodeString(input.TokenHash)
	if err != nil || len(hash) != sha256.Size || hex.EncodeToString(hash) != input.TokenHash {
		http.Error(w, "invalid token fingerprint", http.StatusBadRequest)
		return
	}
	var idBytes [32]byte
	var codeBytes [4]byte
	if _, err := rand.Read(idBytes[:]); err != nil {
		http.Error(w, "pairing unavailable", http.StatusServiceUnavailable)
		return
	}
	if _, err := rand.Read(codeBytes[:]); err != nil {
		http.Error(w, "pairing unavailable", http.StatusServiceUnavailable)
		return
	}
	now := time.Now()
	p.mu.Lock()
	p.expire(now)
	pending := 0
	for _, request := range p.requests {
		if request.Status == "pending" {
			pending++
		}
	}
	if pending >= maxPendingPairings {
		p.mu.Unlock()
		http.Error(w, "too many pending connections", http.StatusTooManyRequests)
		return
	}
	request := &pairingRequest{ID: hex.EncodeToString(idBytes[:]), Code: hex.EncodeToString(codeBytes[:]), Device: input.Device, Expires: now.Add(pairingLifetime), Status: "pending"}
	copy(request.Hash[:], hash)
	p.requests[request.ID] = request
	p.mu.Unlock()
	pairingJSON(w, map[string]any{"request_id": request.ID, "code": request.Code, "expires_in": int(pairingLifetime.Seconds())}, http.StatusCreated)
}

func (p *pairingManager) poll(w http.ResponseWriter, r *http.Request, origin string) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if header := r.Header.Get("Origin"); header != "" && header != origin {
		http.Error(w, "invalid Origin header", http.StatusForbidden)
		return
	}
	var input struct {
		RequestID string `json:"request_id"`
	}
	if readPairingJSON(w, r, &input) != nil || len(input.RequestID) != 64 {
		http.Error(w, "invalid pairing request", http.StatusBadRequest)
		return
	}
	p.mu.Lock()
	request := p.requests[input.RequestID]
	if request == nil {
		p.mu.Unlock()
		http.Error(w, "connection request not found", http.StatusNotFound)
		return
	}
	if !time.Now().Before(request.Expires) {
		delete(p.requests, input.RequestID)
		p.mu.Unlock()
		http.Error(w, "connection request expired", http.StatusGone)
		return
	}
	if request.Status == "denied" {
		p.mu.Unlock()
		http.Error(w, "connection denied", http.StatusForbidden)
		return
	}
	result := request.Status
	status := http.StatusAccepted
	if result == "approved" {
		status = http.StatusOK
	}
	p.mu.Unlock()
	pairingJSON(w, map[string]string{"status": result}, status)
}

func (p *pairingManager) adminPairings(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		p.mu.Lock()
		p.expire(time.Now())
		items := []map[string]string{}
		for _, request := range p.requests {
			if request.Status == "pending" {
				items = append(items, map[string]string{"request_id": request.ID, "code": request.Code, "device": request.Device})
			}
		}
		p.mu.Unlock()
		sort.Slice(items, func(i, j int) bool { return items[i]["code"] < items[j]["code"] })
		pairingJSON(w, items, http.StatusOK)
	case http.MethodPost:
		var input struct {
			RequestID string `json:"request_id"`
			Code      string `json:"code"`
			Decision  string `json:"decision"`
		}
		if readPairingJSON(w, r, &input) != nil || (input.Decision != "approve" && input.Decision != "deny") {
			http.Error(w, "invalid decision", http.StatusBadRequest)
			return
		}
		p.mu.Lock()
		request := p.requests[input.RequestID]
		if request == nil || !time.Now().Before(request.Expires) || request.Status != "pending" {
			p.mu.Unlock()
			http.Error(w, "connection request unavailable", http.StatusNotFound)
			return
		}
		if request.Code != input.Code {
			p.mu.Unlock()
			http.Error(w, "code does not match", http.StatusBadRequest)
			return
		}
		if input.Decision == "approve" {
			if _, err := p.store.AddClientToken(r.Context(), request.Device, request.Hash); err != nil {
				p.mu.Unlock()
				http.Error(w, "connection could not be approved", http.StatusServiceUnavailable)
				return
			}
			request.Status = "approved"
		} else {
			request.Status = "denied"
		}
		p.mu.Unlock()
		w.WriteHeader(http.StatusNoContent)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (p *pairingManager) adminDevices(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		items, err := p.store.ListClientTokens(r.Context())
		if err != nil {
			http.Error(w, "devices unavailable", http.StatusServiceUnavailable)
			return
		}
		pairingJSON(w, items, http.StatusOK)
	case http.MethodDelete:
		var input struct {
			ID int64 `json:"id"`
		}
		if readPairingJSON(w, r, &input) != nil || input.ID <= 0 {
			http.Error(w, "invalid device", http.StatusBadRequest)
			return
		}
		changed, err := p.store.RevokeClientToken(r.Context(), input.ID)
		if err != nil {
			http.Error(w, "revoke unavailable", http.StatusServiceUnavailable)
			return
		}
		if !changed {
			http.Error(w, "device not found", http.StatusNotFound)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

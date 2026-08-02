package handler

import (
	"crypto/subtle"
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/deepaksingh/agent-nexus/services/api/internal/config"
	"github.com/deepaksingh/agent-nexus/services/api/pkg/encrypt"
	"github.com/deepaksingh/agent-nexus/services/api/pkg/errs"
)

// WhatsAppCredsHandler stores and retrieves Baileys auth files so the adapter
// survives container restarts without requiring a new QR scan.
//
// These endpoints carry no JWT (the adapter is a Node process, not a user), but
// they are NOT unauthenticated: every one requires WHATSAPP_INTERNAL_TOKEN via
// X-WhatsApp-Token. Get decrypts and returns live session credentials, so an
// open endpoint here is a full WhatsApp account takeover for anyone who can
// reach the API — network-topology "internal only" was never enforced.
// The token is auto-generated per container in start-api.sh, which runs the API
// and the adapter side by side, so both processes share it with no config.
type WhatsAppCredsHandler struct {
	pool *pgxpool.Pool
	cfg  *config.Config
}

func NewWhatsAppCredsHandler(pool *pgxpool.Pool, cfg *config.Config) *WhatsAppCredsHandler {
	return &WhatsAppCredsHandler{pool: pool, cfg: cfg}
}

// authorized reports whether the request carries the adapter shared secret.
// Fails closed when the secret is unconfigured, and reports 404 rather than 401
// so the endpoint's existence isn't confirmed — same contract as SessionCallback.
func (h *WhatsAppCredsHandler) authorized(w http.ResponseWriter, r *http.Request) bool {
	if h.cfg.WhatsAppInternalToken == "" {
		errs.Write(w, errs.NotFound("whatsapp credential storage is not configured"))
		return false
	}
	if subtle.ConstantTimeCompare([]byte(r.Header.Get("X-WhatsApp-Token")), []byte(h.cfg.WhatsAppInternalToken)) != 1 {
		errs.Write(w, errs.Unauthorized("invalid whatsapp internal token"))
		return false
	}
	return true
}

func (h *WhatsAppCredsHandler) Get(w http.ResponseWriter, r *http.Request) {
	if !h.authorized(w, r) {
		return
	}
	accountID := chi.URLParam(r, "accountId")
	var encrypted string
	if err := h.pool.QueryRow(r.Context(),
		`SELECT data FROM whatsapp_credentials WHERE account_id=$1`, accountID).Scan(&encrypted); err != nil {
		errs.Write(w, errs.NotFound("no credentials stored"))
		return
	}
	plain, err := encrypt.Decrypt([]byte(h.cfg.EncryptionKey), encrypted)
	if err != nil {
		errs.Write(w, errs.Internal("failed to decrypt credentials"))
		return
	}
	var files map[string]any
	if err := json.Unmarshal([]byte(plain), &files); err != nil {
		errs.Write(w, errs.Internal("failed to parse credentials"))
		return
	}
	errs.WriteJSON(w, http.StatusOK, map[string]any{"files": files})
}

func (h *WhatsAppCredsHandler) Put(w http.ResponseWriter, r *http.Request) {
	if !h.authorized(w, r) {
		return
	}
	accountID := chi.URLParam(r, "accountId")
	var body struct {
		Files map[string]any `json:"files"`
	}
	if json.NewDecoder(r.Body).Decode(&body) != nil || len(body.Files) == 0 {
		errs.Write(w, errs.BadRequest("files is required"))
		return
	}
	raw, err := json.Marshal(body.Files)
	if err != nil {
		errs.Write(w, errs.Internal("failed to serialize files"))
		return
	}
	encrypted, err := encrypt.Encrypt([]byte(h.cfg.EncryptionKey), string(raw))
	if err != nil {
		errs.Write(w, errs.Internal("failed to encrypt credentials"))
		return
	}
	if _, err := h.pool.Exec(r.Context(),
		`INSERT INTO whatsapp_credentials(account_id, data, updated_at)
		 VALUES($1, $2, NOW())
		 ON CONFLICT(account_id) DO UPDATE SET data=EXCLUDED.data, updated_at=NOW()`,
		accountID, encrypted); err != nil {
		errs.Write(w, errs.Internal("failed to store credentials"))
		return
	}
	errs.WriteJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (h *WhatsAppCredsHandler) Delete(w http.ResponseWriter, r *http.Request) {
	if !h.authorized(w, r) {
		return
	}
	accountID := chi.URLParam(r, "accountId")
	h.pool.Exec(r.Context(), `DELETE FROM whatsapp_credentials WHERE account_id=$1`, accountID) //nolint:errcheck
	errs.WriteJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// PutLIDMap receives a batch of {jid, lid} pairs from the WhatsApp adapter and
// updates gateway_contacts.whatsapp_lid so the pairing policy can match @lid senders.
func (h *WhatsAppCredsHandler) PutLIDMap(w http.ResponseWriter, r *http.Request) {
	if !h.authorized(w, r) {
		return
	}
	accountID := chi.URLParam(r, "accountId")
	var body struct {
		Contacts []struct {
			JID string `json:"jid"`
			LID string `json:"lid"`
		} `json:"contacts"`
	}
	if json.NewDecoder(r.Body).Decode(&body) != nil {
		errs.WriteJSON(w, http.StatusOK, map[string]any{"ok": true, "updated": 0})
		return
	}
	var updated int64
	for _, c := range body.Contacts {
		if c.JID == "" || c.LID == "" {
			continue
		}
		tag, _ := h.pool.Exec(r.Context(),
			`UPDATE gateway_contacts SET whatsapp_lid=$1, updated_at=NOW()
			 WHERE account_id=$2 AND whatsapp_jid=$3 AND (whatsapp_lid='' OR whatsapp_lid=$1)`,
			c.LID, accountID, c.JID)
		updated += tag.RowsAffected()
	}
	errs.WriteJSON(w, http.StatusOK, map[string]any{"ok": true, "updated": updated})
}

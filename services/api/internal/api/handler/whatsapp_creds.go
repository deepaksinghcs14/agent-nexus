package handler

import (
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
// These endpoints are internal-only (/internal/whatsapp/...) — no JWT auth.
type WhatsAppCredsHandler struct {
	pool *pgxpool.Pool
	cfg  *config.Config
}

func NewWhatsAppCredsHandler(pool *pgxpool.Pool, cfg *config.Config) *WhatsAppCredsHandler {
	return &WhatsAppCredsHandler{pool: pool, cfg: cfg}
}

func (h *WhatsAppCredsHandler) Get(w http.ResponseWriter, r *http.Request) {
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
	accountID := chi.URLParam(r, "accountId")
	h.pool.Exec(r.Context(), `DELETE FROM whatsapp_credentials WHERE account_id=$1`, accountID) //nolint:errcheck
	errs.WriteJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// PutLIDMap receives a batch of {jid, lid} pairs from the WhatsApp adapter and
// updates gateway_contacts.whatsapp_lid so the pairing policy can match @lid senders.
func (h *WhatsAppCredsHandler) PutLIDMap(w http.ResponseWriter, r *http.Request) {
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

package handler

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/deepaksingh/agent-nexus/services/api/internal/api/middleware"
	"github.com/deepaksingh/agent-nexus/services/api/internal/config"
	"github.com/deepaksingh/agent-nexus/services/api/internal/domain" //nolint
	"github.com/deepaksingh/agent-nexus/services/api/internal/repository"
	"github.com/deepaksingh/agent-nexus/services/api/pkg/errs"
)

type ConversationsHandler struct {
	pool          *pgxpool.Pool
	cfg           *config.Config
	conversations *repository.ConversationRepository
}

func NewConversationsHandler(pool *pgxpool.Pool, cfg *config.Config) *ConversationsHandler {
	return &ConversationsHandler{
		pool:          pool,
		cfg:           cfg,
		conversations: repository.NewConversationRepository(pool),
	}
}

func (h *ConversationsHandler) List(w http.ResponseWriter, r *http.Request) {
	wsID := middleware.WorkspaceIDFromCtx(r.Context())
	list, err := h.conversations.List(r.Context(), wsID)
	if err != nil {
		errs.Write(w, errs.Internal("failed to list conversations"))
		return
	}
	errs.WriteJSON(w, http.StatusOK, map[string]any{"data": list})
}

func (h *ConversationsHandler) Create(w http.ResponseWriter, r *http.Request) {
	wsID := middleware.WorkspaceIDFromCtx(r.Context())
	userID := middleware.UserIDFromCtx(r.Context())

	var req struct {
		AgentID string `json:"agent_id"`
		Title   string `json:"title"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		errs.Write(w, errs.BadRequest("invalid request body"))
		return
	}
	if req.AgentID == "" {
		errs.Write(w, errs.BadRequest("agent_id is required"))
		return
	}
	if req.Title == "" {
		req.Title = "New Conversation"
	}

	c := &domain.Conversation{
		ID:          uuid.New().String(),
		WorkspaceID: wsID,
		AgentID:     req.AgentID,
		UserID:      userID,
		Title:       req.Title,
	}
	if err := h.conversations.Create(r.Context(), c); err != nil {
		errs.Write(w, errs.Internal("failed to create conversation"))
		return
	}
	errs.WriteJSON(w, http.StatusCreated, c)
}

func (h *ConversationsHandler) Get(w http.ResponseWriter, r *http.Request) {
	wsID := middleware.WorkspaceIDFromCtx(r.Context())
	c, err := h.conversations.Get(r.Context(), chi.URLParam(r, "id"), wsID)
	if err != nil {
		errs.Write(w, errs.NotFound("conversation not found"))
		return
	}
	msgs, _ := h.conversations.ListMessages(r.Context(), c.ID)
	errs.WriteJSON(w, http.StatusOK, map[string]any{
		"conversation": c,
		"messages":     msgs,
	})
}

func (h *ConversationsHandler) Delete(w http.ResponseWriter, r *http.Request) {
	wsID := middleware.WorkspaceIDFromCtx(r.Context())
	if err := h.conversations.Delete(r.Context(), chi.URLParam(r, "id"), wsID); err != nil {
		errs.Write(w, errs.Internal("failed to delete conversation"))
		return
	}
	w.WriteHeader(http.StatusNoContent)
}


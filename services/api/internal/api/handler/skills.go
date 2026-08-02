package handler

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/deepaksingh/agent-nexus/services/api/internal/api/middleware"
	"github.com/deepaksingh/agent-nexus/services/api/internal/config"
	"github.com/deepaksingh/agent-nexus/services/api/internal/domain"
	"github.com/deepaksingh/agent-nexus/services/api/internal/repository"
	"github.com/deepaksingh/agent-nexus/services/api/pkg/errs"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type SkillsHandler struct {
	pool *pgxpool.Pool
	cfg  *config.Config
	repo *repository.SkillRepository
}

func NewSkillsHandler(pool *pgxpool.Pool, cfg *config.Config) *SkillsHandler {
	return &SkillsHandler{pool: pool, cfg: cfg, repo: repository.NewSkillRepository(pool)}
}

func (h *SkillsHandler) List(w http.ResponseWriter, r *http.Request) {
	ws := middleware.WorkspaceIDFromCtx(r.Context())
	list, err := h.repo.List(r.Context(), ws)
	if err != nil {
		errs.Write(w, errs.Internal("failed to list skills"))
		return
	}
	errs.WriteJSON(w, http.StatusOK, map[string]any{"data": list})
}

func (h *SkillsHandler) Create(w http.ResponseWriter, r *http.Request) {
	ws := middleware.WorkspaceIDFromCtx(r.Context())
	uid := middleware.UserIDFromCtx(r.Context())
	var body struct {
		Name               string   `json:"name"`
		Description        string   `json:"description"`
		Content            string   `json:"content"`
		Enabled            *bool    `json:"enabled"`
		Category           string   `json:"category"`
		RequiredToolNames  []string `json:"required_tool_names"`
	}
	if json.NewDecoder(r.Body).Decode(&body) != nil || body.Name == "" {
		errs.Write(w, errs.BadRequest("name is required"))
		return
	}
	enabled := true
	if body.Enabled != nil {
		enabled = *body.Enabled
	}
	s := &domain.Skill{
		ID:                uuid.NewString(),
		WorkspaceID:       ws,
		Name:              body.Name,
		Description:       body.Description,
		Content:           body.Content,
		Source:            "manual",
		Enabled:           enabled,
		Category:          body.Category,
		RequiredToolNames: body.RequiredToolNames,
		CreatedBy:         uid,
	}
	if err := h.repo.Create(r.Context(), s); err != nil {
		errs.Write(w, errs.Internal("failed to create skill"))
		return
	}
	writeAudit(r, h.pool, "skill.created", "skill", s.ID)
	errs.WriteJSON(w, http.StatusCreated, s)
}

func (h *SkillsHandler) Get(w http.ResponseWriter, r *http.Request) {
	ws := middleware.WorkspaceIDFromCtx(r.Context())
	s, err := h.repo.Get(r.Context(), chi.URLParam(r, "id"), ws)
	if err != nil {
		errs.Write(w, errs.NotFound("skill not found"))
		return
	}
	errs.WriteJSON(w, http.StatusOK, s)
}

func (h *SkillsHandler) Update(w http.ResponseWriter, r *http.Request) {
	ws := middleware.WorkspaceIDFromCtx(r.Context())
	id := chi.URLParam(r, "id")
	s, err := h.repo.Get(r.Context(), id, ws)
	if err != nil {
		errs.Write(w, errs.NotFound("skill not found"))
		return
	}
	if s.Source == "managed" {
		errs.Write(w, errs.Forbidden("managed skills cannot be edited"))
		return
	}
	var body struct {
		Name              *string  `json:"name"`
		Description       *string  `json:"description"`
		Content           *string  `json:"content"`
		Enabled           *bool    `json:"enabled"`
		Category          *string  `json:"category"`
		RequiredToolNames *[]string `json:"required_tool_names"`
	}
	if json.NewDecoder(r.Body).Decode(&body) != nil {
		errs.Write(w, errs.BadRequest("invalid request body"))
		return
	}
	if body.Name != nil {
		s.Name = *body.Name
	}
	if body.Description != nil {
		s.Description = *body.Description
	}
	if body.Content != nil {
		s.Content = *body.Content
	}
	if body.Enabled != nil {
		s.Enabled = *body.Enabled
	}
	if body.Category != nil {
		s.Category = *body.Category
	}
	if body.RequiredToolNames != nil {
		s.RequiredToolNames = *body.RequiredToolNames
	}
	s.WorkspaceID = ws
	if err := h.repo.Update(r.Context(), &s); err != nil {
		errs.Write(w, errs.Internal("failed to update skill"))
		return
	}
	writeAudit(r, h.pool, "skill.updated", "skill", id)
	errs.WriteJSON(w, http.StatusOK, s)
}

func (h *SkillsHandler) Delete(w http.ResponseWriter, r *http.Request) {
	ws := middleware.WorkspaceIDFromCtx(r.Context())
	id := chi.URLParam(r, "id")
	s, err := h.repo.Get(r.Context(), id, ws)
	if err != nil {
		errs.Write(w, errs.NotFound("skill not found"))
		return
	}
	if s.Source == "managed" {
		errs.Write(w, errs.Forbidden("managed skills cannot be deleted"))
		return
	}
	if err := h.repo.Delete(r.Context(), id, ws); err != nil {
		errs.Write(w, errs.Internal("failed to delete skill"))
		return
	}
	writeAudit(r, h.pool, "skill.deleted", "skill", id)
	w.WriteHeader(http.StatusNoContent)
}

func (h *SkillsHandler) ListForAgent(w http.ResponseWriter, r *http.Request) {
	ws := middleware.WorkspaceIDFromCtx(r.Context())
	agentID := chi.URLParam(r, "id")
	list, err := h.repo.ListForAgent(r.Context(), agentID, ws)
	if err != nil {
		errs.Write(w, errs.Internal("failed to list agent skills"))
		return
	}
	errs.WriteJSON(w, http.StatusOK, map[string]any{"data": list})
}

func (h *SkillsHandler) SetForAgent(w http.ResponseWriter, r *http.Request) {
	ws := middleware.WorkspaceIDFromCtx(r.Context())
	agentID := chi.URLParam(r, "id")
	var body struct {
		Skills []domain.AgentSkillAssignment `json:"skills"`
	}
	if json.NewDecoder(r.Body).Decode(&body) != nil {
		errs.Write(w, errs.BadRequest("invalid request body"))
		return
	}
	if err := h.repo.SetForAgent(r.Context(), agentID, ws, body.Skills); err != nil {
		// A foreign (or missing) agent id fails the ownership guard — report it
		// as not-found rather than an internal error, matching the file's style.
		if errors.Is(err, pgx.ErrNoRows) {
			errs.Write(w, errs.NotFound("agent not found"))
			return
		}
		errs.Write(w, errs.Internal("failed to update agent skills"))
		return
	}
	writeAudit(r, h.pool, "agent.skills_updated", "agent", agentID)
	errs.WriteJSON(w, http.StatusOK, map[string]any{"data": body.Skills})
}

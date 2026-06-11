package handler

import (
	"net/http"

	"github.com/deepaksingh/agent-nexus/services/api/internal/config"
	"github.com/deepaksingh/agent-nexus/services/api/pkg/errs"
)

type ConfigHandler struct {
	cfg *config.Config
}

func NewConfigHandler(cfg *config.Config) *ConfigHandler {
	return &ConfigHandler{cfg: cfg}
}

func (h *ConfigHandler) Get(w http.ResponseWriter, r *http.Request) {
	errs.WriteJSON(w, http.StatusOK, map[string]any{
		"demo_mode": h.cfg.DemoMode,
	})
}

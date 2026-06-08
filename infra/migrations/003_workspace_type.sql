-- Migration 003: Add workspace_type column
ALTER TABLE workspaces
  ADD COLUMN workspace_type TEXT NOT NULL DEFAULT 'personal'
    CHECK (workspace_type IN ('personal','team','organization','project','sandbox'));

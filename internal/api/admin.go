package api

import (
	"encoding/json"
	"net/http"

	"github.com/b-j-roberts/ibis/internal/config"
)

// adminAuth wraps a handler with optional API key authentication.
// If no admin key is configured, all requests are allowed.
func (s *Server) adminAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if s.cfg.AdminKey != "" {
			key := r.Header.Get("X-Admin-Key")
			if key != s.cfg.AdminKey {
				writeError(w, http.StatusUnauthorized, "invalid or missing admin key")
				return
			}
		}
		next(w, r)
	}
}

// handleAdminRegisterContract handles POST /v1/admin/contracts.
func (s *Server) handleAdminRegisterContract(w http.ResponseWriter, r *http.Request) {
	if s.engine == nil {
		writeError(w, http.StatusServiceUnavailable, "dynamic registration not available")
		return
	}

	var cc config.ContractConfig
	if err := json.NewDecoder(r.Body).Decode(&cc); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body: "+err.Error())
		return
	}

	if cc.Name == "" {
		writeError(w, http.StatusBadRequest, "contract name is required")
		return
	}
	if cc.Address == "" {
		writeError(w, http.StatusBadRequest, "contract address is required")
		return
	}

	if err := s.engine.RegisterContract(r.Context(), &cc); err != nil {
		writeError(w, http.StatusInternalServerError, "registration failed: "+err.Error())
		return
	}

	writeJSON(w, http.StatusCreated, map[string]any{
		"status":  "registered",
		"name":    cc.Name,
		"address": cc.Address,
	})
}

// handleAdminDeregisterContract handles DELETE /v1/admin/contracts/{name}.
func (s *Server) handleAdminDeregisterContract(w http.ResponseWriter, r *http.Request) {
	if s.engine == nil {
		writeError(w, http.StatusServiceUnavailable, "dynamic registration not available")
		return
	}

	name := r.PathValue("name")
	if name == "" {
		writeError(w, http.StatusBadRequest, "contract name is required")
		return
	}

	dropTables := r.URL.Query().Get("drop_tables") == "true"

	if err := s.engine.DeregisterContract(r.Context(), name, dropTables); err != nil {
		writeError(w, http.StatusInternalServerError, "deregistration failed: "+err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"status":      "deregistered",
		"name":        name,
		"drop_tables": dropTables,
	})
}

// handleAdminListContracts handles GET /v1/admin/contracts.
func (s *Server) handleAdminListContracts(w http.ResponseWriter, r *http.Request) {
	if s.engine == nil {
		writeError(w, http.StatusServiceUnavailable, "dynamic registration not available")
		return
	}

	contracts := s.engine.Contracts(r.Context())
	writeJSON(w, http.StatusOK, map[string]any{
		"contracts": contracts,
		"count":     len(contracts),
	})
}

// handleAdminUpdateContract handles PUT /v1/admin/contracts/{name}.
func (s *Server) handleAdminUpdateContract(w http.ResponseWriter, r *http.Request) {
	if s.engine == nil {
		writeError(w, http.StatusServiceUnavailable, "dynamic registration not available")
		return
	}

	name := r.PathValue("name")
	if name == "" {
		writeError(w, http.StatusBadRequest, "contract name is required")
		return
	}

	// Verify contract exists.
	existing := s.engine.FindContract(name)
	if existing == nil {
		writeError(w, http.StatusNotFound, "contract not found: "+name)
		return
	}

	var cc config.ContractConfig
	if err := json.NewDecoder(r.Body).Decode(&cc); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body: "+err.Error())
		return
	}

	if err := s.engine.UpdateContract(r.Context(), name, &cc); err != nil {
		writeError(w, http.StatusInternalServerError, "update failed: "+err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"status": "updated",
		"name":   name,
	})
}

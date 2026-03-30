package api

import (
	"net/http"
)

// handleDiscoverContracts returns contracts discovered via class hash watching
// for a specific class hash.
func (s *Server) handleDiscoverContracts(w http.ResponseWriter, r *http.Request) {
	if s.engine == nil {
		writeError(w, http.StatusServiceUnavailable, "engine not available")
		return
	}

	classHash := r.PathValue("classHash")
	if classHash == "" {
		writeError(w, http.StatusBadRequest, "classHash path parameter required")
		return
	}

	contracts := s.engine.DiscoveredContractsByClassHash(classHash)
	writeJSON(w, http.StatusOK, contracts)
}

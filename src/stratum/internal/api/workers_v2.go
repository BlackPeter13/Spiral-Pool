// SPDX-License-Identifier: BSD-3-Clause
// SPDX-FileCopyrightText: Copyright (c) 2026 Spiral Pool Contributors

// Package api provides V2 worker statistics API endpoints.
//
// These handlers mirror the V1 worker endpoints but operate on pool-scoped
// database tables via WithPoolID(). All endpoints use the database directly
// (no WorkerStatsProvider injection) since V2 CoinPools share the DB layer.
package api

import (
	"net/http"
	"strconv"

	"github.com/spiralpool/stratum/internal/database"
)

// handleMinerWorkersV2 handles GET /api/pools/{id}/miners/{address}/workers
// Returns all workers for a specific miner, scoped to the pool's DB tables.
func (s *ServerV2) handleMinerWorkersV2(w http.ResponseWriter, r *http.Request, poolID, address string) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if !validAddressPattern.MatchString(address) {
		http.Error(w, "Invalid miner address format", http.StatusBadRequest)
		return
	}

	// Parse window parameter (default: 15 minutes for efficiency)
	windowMinutes := 15
	if wParam := r.URL.Query().Get("window"); wParam != "" {
		if parsed, err := strconv.Atoi(wParam); err == nil && parsed >= 1 && parsed <= 1440 {
			windowMinutes = parsed
		}
	}

	if s.db == nil {
		http.Error(w, "Database unavailable", http.StatusServiceUnavailable)
		return
	}

	scopedDB := s.db.WithPoolID(poolID)
	workers, err := scopedDB.GetMinerWorkers(r.Context(), address, windowMinutes)
	if err != nil {
		s.logger.Errorw("Failed to get miner workers", "poolId", poolID, "error", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	response := make([]*WorkerSummaryResponse, 0, len(workers))
	for _, wr := range workers {
		response = append(response, &WorkerSummaryResponse{
			Miner:          wr.Miner,
			Worker:         wr.Worker,
			Hashrate:       wr.Hashrate,
			SharesPerSec:   wr.SharesPerSec,
			AcceptanceRate: wr.AcceptanceRate,
			LastShare:      wr.LastShare.Format("2006-01-02T15:04:05Z"),
			Connected:      wr.Connected,
		})
	}

	s.writeJSON(w, response)
}

// handleWorkerStatsV2 handles GET /api/pools/{id}/miners/{address}/workers/{worker}
// Returns detailed statistics for a specific worker.
func (s *ServerV2) handleWorkerStatsV2(w http.ResponseWriter, r *http.Request, poolID, address, worker string) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if !validAddressPattern.MatchString(address) {
		http.Error(w, "Invalid miner address format", http.StatusBadRequest)
		return
	}
	if worker != "" && !validWorkerPattern.MatchString(worker) {
		http.Error(w, "Invalid worker name format", http.StatusBadRequest)
		return
	}

	if worker == "" {
		worker = "default"
	}

	if s.db == nil {
		http.Error(w, "Database unavailable", http.StatusServiceUnavailable)
		return
	}

	scopedDB := s.db.WithPoolID(poolID)
	stats, err := scopedDB.GetWorkerStats(r.Context(), address, worker, 1440) // 24h window
	if err != nil {
		s.logger.Errorw("Failed to get worker stats", "poolId", poolID, "error", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}
	if stats == nil {
		http.Error(w, "Worker not found", http.StatusNotFound)
		return
	}

	response := &WorkerStatsResponse{
		Miner:           stats.Miner,
		Worker:          stats.Worker,
		CurrentHashrate: stats.Hashrate,
		SharesSubmitted: stats.SharesSubmitted,
		SharesAccepted:  stats.SharesAccepted,
		SharesRejected:  stats.SharesRejected,
		AcceptanceRate:  stats.AcceptanceRate,
		LastShare:       stats.LastShare.Format("2006-01-02T15:04:05Z"),
	}

	s.writeJSON(w, response)
}

// handleWorkerHistoryV2 handles GET /api/pools/{id}/miners/{address}/workers/{worker}/history
// Returns hashrate history for graphs.
func (s *ServerV2) handleWorkerHistoryV2(w http.ResponseWriter, r *http.Request, poolID, address, worker string) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if !validAddressPattern.MatchString(address) {
		http.Error(w, "Invalid miner address format", http.StatusBadRequest)
		return
	}
	if worker != "" && !validWorkerPattern.MatchString(worker) {
		http.Error(w, "Invalid worker name format", http.StatusBadRequest)
		return
	}

	if worker == "" {
		worker = "default"
	}

	// Parse hours parameter (default: 24, max: 720 = 30 days)
	hours := 24
	if h := r.URL.Query().Get("hours"); h != "" {
		if parsed, err := strconv.Atoi(h); err == nil && parsed > 0 && parsed <= 720 {
			hours = parsed
		}
	}

	if s.db == nil {
		http.Error(w, "Database unavailable", http.StatusServiceUnavailable)
		return
	}

	scopedDB := s.db.WithPoolID(poolID)
	history, err := scopedDB.GetWorkerHashrateHistory(r.Context(), address, worker, hours)
	if err != nil {
		s.logger.Errorw("Failed to get worker history", "poolId", poolID, "error", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	response := make([]*HashratePointResponse, 0, len(history))
	for _, h := range history {
		response = append(response, &HashratePointResponse{
			Timestamp: h.Timestamp.Format("2006-01-02T15:04:05Z"),
			Hashrate:  h.Hashrate,
			Window:    h.Window,
		})
	}

	s.writeJSON(w, response)
}

// handlePoolHashrateHistoryV2 handles GET /api/pools/{id}/hashrate/history
// Returns pool-wide hashrate history for graphs.
func (s *ServerV2) handlePoolHashrateHistoryV2(w http.ResponseWriter, r *http.Request, poolID string) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Parse hours parameter (default: 24, max: 720 = 30 days)
	hours := 24
	if h := r.URL.Query().Get("hours"); h != "" {
		if parsed, err := strconv.Atoi(h); err == nil && parsed > 0 && parsed <= 720 {
			hours = parsed
		}
	}

	if s.db == nil {
		http.Error(w, "Database unavailable", http.StatusServiceUnavailable)
		return
	}

	scopedDB := s.db.WithPoolID(poolID)
	history, err := scopedDB.GetPoolHashrateHistory(r.Context(), hours)
	if err != nil {
		s.logger.Errorw("Failed to get pool hashrate history", "poolId", poolID, "error", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	response := make([]*HashratePointResponse, 0, len(history))
	for _, h := range history {
		response = append(response, &HashratePointResponse{
			Timestamp: h.Timestamp.Format("2006-01-02T15:04:05Z"),
			Hashrate:  h.Hashrate,
			Window:    h.Window,
		})
	}

	s.writeJSON(w, response)
}

// handlePoolMinersV2 handles GET /api/pools/{id}/miners
// Returns all active miners for a pool.
func (s *ServerV2) handlePoolMinersV2(w http.ResponseWriter, r *http.Request, poolID string) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if s.db == nil {
		http.Error(w, "Database unavailable", http.StatusServiceUnavailable)
		return
	}

	scopedDB := s.db.WithPoolID(poolID)
	miners, err := scopedDB.GetActiveMiners(r.Context(), 10)
	if err != nil {
		s.logger.Errorw("Failed to get active miners", "poolId", poolID, "error", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	if miners == nil {
		miners = []*database.MinerSummary{}
	}

	s.writeJSON(w, miners)
}

// handleMinerStatsV2 handles GET /api/pools/{id}/miners/{address}
// Returns statistics for a specific miner.
func (s *ServerV2) handleMinerStatsV2(w http.ResponseWriter, r *http.Request, poolID, address string) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if !validAddressPattern.MatchString(address) {
		http.Error(w, "Invalid miner address format", http.StatusBadRequest)
		return
	}

	if s.db == nil {
		http.Error(w, "Database unavailable", http.StatusServiceUnavailable)
		return
	}

	scopedDB := s.db.WithPoolID(poolID)
	stats, err := scopedDB.GetMinerStats(r.Context(), address)
	if err != nil {
		s.logger.Errorw("Failed to get miner stats", "poolId", poolID, "error", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}
	if stats == nil {
		http.Error(w, "Miner not found", http.StatusNotFound)
		return
	}

	response := map[string]interface{}{
		"address":         stats.Address,
		"hashrate":        stats.Hashrate,
		"sharesPerSecond": float64(stats.ShareCount) / (24 * 3600),
		"lastShare":       stats.LastShare,
	}

	s.writeJSON(w, response)
}

// handlePoolWorkersV2 handles GET /api/pools/{id}/workers (admin)
// Returns all workers across all miners (requires auth).
func (s *ServerV2) handlePoolWorkersV2(w http.ResponseWriter, r *http.Request, poolID string) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Parse limit parameter (default: 100, max: 1000)
	limit := 100
	if l := r.URL.Query().Get("limit"); l != "" {
		if parsed, err := strconv.Atoi(l); err == nil && parsed > 0 && parsed <= 1000 {
			limit = parsed
		}
	}

	if s.db == nil {
		http.Error(w, "Database unavailable", http.StatusServiceUnavailable)
		return
	}

	scopedDB := s.db.WithPoolID(poolID)
	workers, err := scopedDB.GetAllWorkers(r.Context(), 15, limit)
	if err != nil {
		s.logger.Errorw("Failed to get all workers", "poolId", poolID, "error", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	response := make([]*WorkerSummaryResponse, 0, len(workers))
	for _, wr := range workers {
		response = append(response, &WorkerSummaryResponse{
			Miner:          wr.Miner,
			Worker:         wr.Worker,
			Hashrate:       wr.Hashrate,
			SharesPerSec:   wr.SharesPerSec,
			AcceptanceRate: wr.AcceptanceRate,
			LastShare:      wr.LastShare.Format("2006-01-02T15:04:05Z"),
			Connected:      wr.Connected,
		})
	}

	s.writeJSON(w, response)
}

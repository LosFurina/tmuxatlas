package server

import (
	"encoding/json"
	"net/http"

	"github.com/LosFurina/tmuxatlas/pkg/common"
)

type RuntimeHealth struct {
	Role       string `json:"role"`
	Deployment string `json:"deployment"`
	Version    string `json:"version"`
	Commit     string `json:"commit"`
	InstanceID string `json:"instance_id"`
	Ready      bool   `json:"ready"`
}

func healthHandler(health RuntimeHealth) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if !health.Ready {
			w.WriteHeader(http.StatusServiceUnavailable)
		}
		_ = json.NewEncoder(w).Encode(health)
	}
}

func nativeHealth(role, instanceID string, ready bool) RuntimeHealth {
	return RuntimeHealth{
		Role: role, Deployment: "native", Version: common.SUMMARY,
		Commit: common.COMMIT, InstanceID: instanceID, Ready: ready,
	}
}

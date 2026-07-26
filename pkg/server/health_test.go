package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRuntimeHealthRolesAndReadiness(t *testing.T) {
	for _, role := range []string{"hub", "standalone", "agent"} {
		t.Run(role, func(t *testing.T) {
			for _, ready := range []bool{false, true} {
				response := httptest.NewRecorder()
				healthHandler(nativeHealth(role, "instance-a", ready)).ServeHTTP(
					response, httptest.NewRequest(http.MethodGet, "/health", nil),
				)
				wantStatus := http.StatusServiceUnavailable
				if ready {
					wantStatus = http.StatusOK
				}
				if response.Code != wantStatus {
					t.Fatalf("status = %d, want %d", response.Code, wantStatus)
				}
				var health RuntimeHealth
				if err := json.NewDecoder(response.Body).Decode(&health); err != nil {
					t.Fatal(err)
				}
				if health.Role != role || health.InstanceID == "" || health.Deployment != "native" {
					t.Fatalf("health = %#v", health)
				}
			}
		})
	}
}

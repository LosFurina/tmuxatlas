package server

import (
	"net"
	"net/http"

	"github.com/LosFurina/tmuxatlas/pkg/ingress"
)

func admissionMiddleware(policy *ingress.Policy, category ingress.Category) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			source := r.RemoteAddr
			if host, _, err := net.SplitHostPort(r.RemoteAddr); err == nil {
				source = host
			}
			lease, err := policy.Acquire(category, source)
			if err != nil {
				http.Error(w, "request admission rejected", ingress.HTTPStatus(err))
				return
			}
			defer lease.Release()
			next.ServeHTTP(w, r)
		})
	}
}

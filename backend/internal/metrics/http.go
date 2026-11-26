package metrics

import (
	"context"
	"net/http"
	"strconv"
	"time"
)

func knownMethod(m string) string {
	switch m {
	case http.MethodGet, http.MethodHead, http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete, http.MethodOptions:
		return m
	}
	return "OTHER"
}

func ObserveHTTP(ctx context.Context, method, route string, code int, took time.Duration) {
	method = knownMethod(method)
	HTTPRequests.WithLabelValues(method, route, strconv.Itoa(code)).Inc()
	Observe(ctx, HTTPDuration.WithLabelValues(method, route), took.Seconds())
}

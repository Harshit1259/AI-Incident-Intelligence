package middleware

import (
	"fmt"
	"net/http"
	"sync/atomic"
	"time"
)

var requestCounter uint64

func RequestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestID := buildRequestID()
		w.Header().Set("X-Request-ID", requestID)

		clonedRequest := r.Clone(r.Context())
		clonedRequest.Header.Set("X-Request-ID", requestID)

		next.ServeHTTP(w, clonedRequest)
	})
}

func buildRequestID() string {
	counter := atomic.AddUint64(&requestCounter, 1)
	return fmt.Sprintf("req-%d-%d", time.Now().UnixNano(), counter)
}

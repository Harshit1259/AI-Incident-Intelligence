package middleware

import (
	"log"
	"net/http"
	"time"
)

type statusRecorder struct {
	http.ResponseWriter
	statusCode int
}

func (r *statusRecorder) WriteHeader(code int) {
	r.statusCode = code
	r.ResponseWriter.WriteHeader(code)
}

func RequestLogger(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()

		rec := &statusRecorder{
			ResponseWriter: w,
			statusCode:     http.StatusOK,
		}

		next.ServeHTTP(rec, r)

		requestID := r.Header.Get("X-Request-ID")
		if requestID == "" {
			requestID = "missing"
		}

		log.Printf(
			"request_id=%s method=%s path=%s status=%d duration=%s",
			requestID,
			r.Method,
			r.URL.RequestURI(),
			rec.statusCode,
			time.Since(start),
		)
	})
}

package main

import (
	"encoding/json"
	"log"
	"net/http"
)

func enableCors(handler http.HandlerFunc) http.HandlerFunc {
	return func(responseWriter http.ResponseWriter, request *http.Request) {
		responseWriter.Header().Set("Access-Control-Allow-Origin", "*")
		responseWriter.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		responseWriter.Header().Set("Access-Control-Allow-Headers", "Content-Type")

		if request.Method == http.MethodOptions {
			responseWriter.WriteHeader(http.StatusOK)
			return
		}

		handler(responseWriter, request)
	}
}

func healthHandler(responseWriter http.ResponseWriter, request *http.Request) {
	responseWriter.Header().Set("Content-Type", "application/json")
	responseWriter.WriteHeader(http.StatusOK)

	response := map[string]string{
		"status":  "ok",
		"service": "backend",
	}

	if err := json.NewEncoder(responseWriter).Encode(response); err != nil {
		http.Error(responseWriter, "failed to encode response", http.StatusInternalServerError)
		return
	}
}

func main() {
	http.HandleFunc("/health", enableCors(healthHandler))

	log.Println("Backend server running on :8080")

	if err := http.ListenAndServe(":8080", nil); err != nil {
		log.Fatal(err)
	}
}

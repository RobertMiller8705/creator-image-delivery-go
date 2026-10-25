package main

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"os"
	"time"
)

func main() {
	key, bucket := os.Getenv("INFRAI_API_KEY"), os.Getenv("IMAGE_BUCKET")
	if key == "" || bucket == "" || !safeSegment(bucket) {
		log.Fatal("set INFRAI_API_KEY and a simple IMAGE_BUCKET name")
	}
	c := client{key, &http.Client{Timeout: 45 * time.Second}}
	if err := c.call(context.Background(), http.MethodPost, "/v1/storage/bucket/create", map[string]any{"name": bucket}, nil); err != nil {
		var api apiError
		if !errors.As(err, &api) || api.Status != http.StatusConflict {
			log.Fatal(err)
		}
	}
	http.HandleFunc("POST /assets", func(w http.ResponseWriter, r *http.Request) {
		var in assetRequest
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&in); err != nil {
			http.Error(w, "invalid JSON", 400)
			return
		}
		result, err := c.deliver(r.Context(), bucket, in)
		if err != nil {
			var api apiError
			status := http.StatusBadGateway
			if errors.As(err, &api) && api.Status >= 400 && api.Status < 500 {
				status = api.Status
			} else if !errors.As(err, &api) {
				status = http.StatusBadRequest
			}
			http.Error(w, err.Error(), status)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(result)
	})
	log.Fatal(http.ListenAndServe(":8080", nil))
}

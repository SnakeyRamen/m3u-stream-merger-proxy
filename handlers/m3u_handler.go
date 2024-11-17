package handlers

import (
	"m3u-stream-merger/store"
	"m3u-stream-merger/utils"
	"net/http"
	"os"
)

func M3UHandler(w http.ResponseWriter, r *http.Request) {
	debug := os.Getenv("DEBUG") == "true"

	w.Header().Set("Content-Type", "text/plain")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	contentStream := make(chan string)

	go store.RevalidatingGetM3U(r, contentStream, false)
	for {
		data, ok := <-contentStream
		if !ok {
			w.WriteHeader(http.StatusOK)
			return
		}

		_, err := w.Write([]byte(data))
		if err != nil {
			if debug {
				utils.SafeLogf("[DEBUG] Error writing http response: %v\n", err)
			}
		}
	}
}

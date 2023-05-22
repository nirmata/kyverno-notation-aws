package main

import (
	"context"
	"encoding/json"
	"io"
	"log"
	"net/http"
)

type RequestData struct {
	Images []string
}

func (v *verifier) handleCheckImages(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)
		return
	}

	var requestData RequestData
	//err := json.NewDecoder(r.Body).Decode(&requestData)
	raw, _ := io.ReadAll(r.Body)
	err := json.Unmarshal(raw, &requestData)
	if err != nil {
		v.logger.Infof("failed to decode %s: %v", string(raw), err)
		http.Error(w, err.Error(), http.StatusNotAcceptable)
		return
	}

	if len(requestData.Images) == 0 {
		log.Printf("missing images in %v", requestData)
		http.Error(w, "missing required parameter 'images'", http.StatusNotAcceptable)
		return
	}

	ctx := context.Background()
	data, err := v.verifyImages(ctx, requestData.Images)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
	w.Write(data)
}

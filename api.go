package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

func Payment(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodPost:
		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, fmt.Sprintf("Error reading body: %v", err), http.StatusBadRequest)
			return
		}
		defer r.Body.Close()
		type CheckRequest struct {
			Id     string `json:"id"`
			Amount int    `json:"amount"`
		}
		var req CheckRequest
		err = json.Unmarshal(body, &req)
		if err != nil {
			http.Error(w, fmt.Sprintf("Invalid JSON: %v", err), http.StatusBadRequest)
			return
		}
		jsonData, err := json.Marshal(req)
		if err != nil {
			http.Error(w, "Failed to marshal JSON", http.StatusInternalServerError)
			return
		}
		response, err := http.Post("http://localhost:3001/test1", "application/json", bytes.NewBuffer(jsonData))
		if err != nil {
			http.Error(w, "Failed to send request", http.StatusInternalServerError)
			return
		}
		defer response.Body.Close()

		var dat map[string]interface{}
		if err := json.NewDecoder(response.Body).Decode(&dat); err != nil {
			http.Error(w, "Failed to decode response", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{
			"status": dat["status"].(string),
		})
		fmt.Printf("%v", dat["status"].(string))
	}
}

func main() {
	http.HandleFunc("/payment", Payment)
	http.ListenAndServe(":3000", nil)
}

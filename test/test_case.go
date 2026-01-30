package main

import (
	"encoding/json"
	"fmt"
	"math/rand/v2"
	"net/http"
)

type returnstatus int

const (
	SUCCESS returnstatus = iota
	PENDING
	FAILED
)

var returnstatusname = map[returnstatus]string{
	SUCCESS: "SUCCESS",
	PENDING: "PENDING",
	FAILED:  "FAILED",
}

func GetRandom(a int) int {
	return rand.IntN(a)
}
func test1(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodPost:
		// body, err := io.ReadAll(r.Body)
		// if err != nil {
		// 	http.Error(w, fmt.Sprintf("Error reading body: %v", err), http.StatusBadRequest)
		// 	return
		// }
		// defer r.Body.Close()
		status := returnstatus(GetRandom(3))
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{
			"status": returnstatusname[status],
		})
	}
}
func main() {
	http.HandleFunc("/test1", test1)
	http.ListenAndServe(":3001", nil)
	fmt.Println("Started at 3001")
}

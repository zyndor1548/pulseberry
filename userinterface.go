package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"golang.org/x/crypto/bcrypt"
)

func HashPassword(password string) (string, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return "", err
	}
	return string(hash), nil
}

func VerifyPassword(password string, hash string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) == nil
}
func RegisterHandler(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodPost:
		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, fmt.Sprintf("Error reading body: %v", err), http.StatusBadRequest)
			return
		}
		defer r.Body.Close()

		type RegisterRequest struct {
			Username string `json:"username"`
			Password string `json:"password"`
		}
		var req RegisterRequest
		err = json.Unmarshal(body, &req)
		if err != nil {
			http.Error(w, fmt.Sprintf("Invalid JSON: %v", err), http.StatusBadRequest)
			return
		}

		if req.Username == "" || req.Password == "" {
			http.Error(w, "No name or password provided", http.StatusBadRequest)
			return
		}
		done := make(chan bool)
		token := make(chan string)
		go CreateUser(req.Username, req.Password, done, token)
		select {
		case <-done:
			tokenString := <-token
			w.Header().Set("Content-Type", "application/json")
			response := fmt.Sprintf(`{"username":"%v","token":"%v"}`, req.Username, tokenString)
			w.Write([]byte(response))
			return
		case <-time.After(5 * time.Second):
			w.Write([]byte(fmt.Sprintf("%v", http.StatusInternalServerError)))
			return
		}
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}
func LoginHandler(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodPost:
		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, fmt.Sprintf("Error reading body: %v", err), http.StatusBadRequest)
			return
		}
		defer r.Body.Close()

		type LoginRequest struct {
			Username string `json:"username"`
			Password string `json:"password"`
		}
		var req LoginRequest
		err = json.Unmarshal(body, &req)
		if err != nil {
			http.Error(w, fmt.Sprintf("Invalid JSON: %v", err), http.StatusBadRequest)
			return
		}

		if req.Username == "" || req.Password == "" {
			http.Error(w, "No name or password provided", http.StatusBadRequest)
			return
		}
		done := make(chan bool)
		token := make(chan string)
		go ValidateUser(req.Username, req.Password, done, token)
		select {
		case <-done:
			tokenString := <-token
			w.Header().Set("Content-Type", "application/json")
			response := fmt.Sprintf(`{"username":"%v","token":"%v"}`, req.Username, tokenString)
			w.Write([]byte(response))
			return
		case <-time.After(5 * time.Second):
			w.Write([]byte(fmt.Sprintf("%v", http.StatusInternalServerError)))
			return
		}
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

func user() {

	_, err := ConnectDatabase()
	if err != nil {
		fmt.Printf("Failed to connect to database: %v\n", err)
		return
	}
	defer DisconnectDatabase()
	http.HandleFunc("/user/register", RegisterHandler)
	http.HandleFunc("/user/login", LoginHandler)
}

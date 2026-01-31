package main

import (
	"database/sql"
	"fmt"
	"os"
	"strings"
	"time"

	_ "github.com/go-sql-driver/mysql"
	"github.com/golang-jwt/jwt/v5"
)

var jwtSecret = []byte(os.Getenv("JWT_SECRET"))

var Databaseconnection *sql.DB

func ConnectDatabase() (*sql.DB, error) {
	mysqlHost := os.Getenv("MYSQL_HOST")
	mysqlPort := os.Getenv("MYSQL_PORT")
	mysqlUsername := os.Getenv("MYSQL_USER")
	mysqlPassword := os.Getenv("MYSQL_PASSWORD")
	mysqlDatabase := os.Getenv("MYSQL_DATABASE")

	connectionString := fmt.Sprintf("%s:%s@tcp(%s:%s)/%s?parseTime=true", mysqlUsername, mysqlPassword, mysqlHost, mysqlPort, mysqlDatabase)

	var err error
	Databaseconnection, err = sql.Open("mysql", connectionString)
	if err != nil {
		return nil, err
	}
	return Databaseconnection, nil
}

func DisconnectDatabase() error {
	if Databaseconnection != nil {
		return Databaseconnection.Close()
	}
	return nil
}
func CreateDatabases() {
	Query := `CREATE TABLE IF NOT EXISTS log(
				id INT AUTO_INCREMENT PRIMARY KEY,
				payment_id VARCHAR(255),
				server_url VARCHAR(255) NOT NULL,
				latency_ms INT NOT NULL,
				success BOOLEAN NOT NULL,
				score FLOAT NOT NULL,
				error_type VARCHAR(50),
				error_message TEXT,
				created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
				INDEX idx_server_url (server_url),
				INDEX idx_created_at (created_at)
				);`
	var err error
	_, err = Databaseconnection.Exec(Query)
	if err != nil {
		fmt.Printf("log table creation failed with error %v\n", err)
	}

	usersQuery := `
	CREATE TABLE IF NOT EXISTS users (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		name TEXT NOT NULL UNIQUE,
		password TEXT NOT NULL,
		email TEXT DEFAULT NULL,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP
	)`

	_, err = Databaseconnection.Exec(usersQuery)
	if err != nil {
		fmt.Printf("user table creation failure %v\n", err)
	}

}

func LogRequestMetrics(paymentID, serverURL string, latencyMs int64, success bool, score float64, errorType, errorMessage string) error {
	if Databaseconnection == nil {
		return fmt.Errorf("database connection is nil")
	}

	query := `INSERT INTO log (payment_id, server_url, latency_ms, success, score, error_type, error_message) 
			  VALUES (?, ?, ?, ?, ?, ?, ?)`

	_, err := Databaseconnection.Exec(query, paymentID, serverURL, latencyMs, success, score, errorType, errorMessage)
	if err != nil {
		return fmt.Errorf("failed to log request metrics: %v", err)
	}

	return nil
}

type LogItem struct {
	TransactionID int    `json:"transaction_id"`
	Link          string `json:"link"`
	Name          string `json:"name"`
	Status        int    `json:"status"`
	Latency       int    `json:"latency"`
	CurrentTime   int64  `json:"current_time"`
}

func GetLogs() ([]LogItem, error) {
	if Databaseconnection == nil {
		return nil, fmt.Errorf("database connection is nil")
	}

	query := `SELECT id, server_url, success, latency_ms, created_at FROM log ORDER BY created_at DESC;`
	rows, err := Databaseconnection.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var logs []LogItem
	for rows.Next() {
		var item LogItem
		var success bool
		var createdAt time.Time
		var serverURL string

		err := rows.Scan(&item.TransactionID, &serverURL, &success, &item.Latency, &createdAt)
		if err != nil {
			return nil, err
		}

		item.Link = serverURL
		// Extract name from server_url (last segment)
		parts := strings.Split(strings.TrimRight(serverURL, "/"), "/")
		if len(parts) > 0 {
			item.Name = parts[len(parts)-1]
		} else {
			item.Name = serverURL
		}

		if success {
			item.Status = 1
		} else {
			item.Status = 0
		}
		item.CurrentTime = createdAt.Unix()

		logs = append(logs, item)
	}

	if err = rows.Err(); err != nil {
		return nil, err
	}

	return logs, nil
}
func ValidateUser(name, password string, done chan bool, token chan string) error {
	query := "SELECT id, name, password FROM users WHERE name = ?"

	var userid int64
	var dbName, dbPassword string

	err := Databaseconnection.QueryRow(query, name).Scan(&userid, &dbName, &dbPassword)
	if err != nil {
		if err == sql.ErrNoRows {
			done <- false
			token <- ""
			return err
		}
		done <- false
		token <- ""
		return err
	}
	if VerifyPassword(password, dbPassword) {
		done <- true
		go GenerateToken(userid, token)
		return nil
	}
	done <- false
	token <- ""
	return nil
}

type Claims struct {
	UserID int64 `json:"user_id"`
	jwt.RegisteredClaims
}

func GenerateToken(userID int64, returnToken chan string) {
	claims := Claims{
		UserID: userID,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(30 * 24 * time.Hour)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenString, err := token.SignedString(jwtSecret)
	if err != nil {
		returnToken <- ""
		return
	}
	returnToken <- tokenString
}

func CreateUser(name, password string, done chan bool, token chan string) error {
	query := "INSERT INTO users (name, password) VALUES (?, ?)"
	hashPassword, err := HashPassword(password)
	if err != nil {
		fmt.Printf("error : %v", err)
		return fmt.Errorf("failed to hash password: %w", err)
	}
	result, err := Databaseconnection.Exec(query, name, hashPassword)
	if err != nil {
		fmt.Printf("error : %v", err)
		return fmt.Errorf("failed to insert server: %w", err)
	}

	lastID, err := result.LastInsertId()
	if err != nil {
		fmt.Printf("error : %v", err)
		return err
	}
	done <- true
	go GenerateToken(lastID, token)
	return nil
}

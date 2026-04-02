package store

import (
	"database/sql"
	"log"

	_ "github.com/lib/pq"
)

func NewDB() *sql.DB {
	connStr := "host=localhost port=5432 user=aiops_user password=aiops_pass dbname=aiops sslmode=disable"

	db, err := sql.Open("postgres", connStr)
	if err != nil {
		log.Fatal("failed to connect to database:", err)
	}

	if err := db.Ping(); err != nil {
		log.Fatal("database not reachable:", err)
	}

	log.Println("Connected to PostgreSQL")

	return db
}

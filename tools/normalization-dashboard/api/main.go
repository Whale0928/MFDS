package main

import (
	"context"
	"database/sql"
	"log"
	"os"
	"time"

	"github.com/bottle-note/mfds-normalization-dashboard-api/dashboard"
	_ "github.com/go-sql-driver/mysql"
)

const listenAddress = "127.0.0.1:8787"

func main() {
	dsn := os.Getenv("MFDS_DEMO_DSN")
	if dsn == "" {
		log.Fatal("MFDS_DEMO_DSN is required for the normalization dashboard API")
	}

	db, err := sql.Open("mysql", dsn)
	if err != nil {
		log.Fatalf("open read-only dashboard database: %v", err)
	}
	defer db.Close()
	db.SetConnMaxLifetime(3 * time.Minute)
	db.SetMaxOpenConns(4)
	db.SetMaxIdleConns(4)
	schemaContext, cancelSchemaCheck := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancelSchemaCheck()
	if err := dashboard.ValidateRequiredSchema(schemaContext, dashboard.SQLQueryer{DB: db}); err != nil {
		log.Fatalf("normalization dashboard schema is not ready: %v", err)
	}

	server := dashboard.NewServer(dashboard.SQLQueryer{DB: db})
	log.Printf("normalization dashboard API listening on http://%s", listenAddress)
	if err := server.ListenAndServe(listenAddress); err != nil {
		log.Fatal(err)
	}

}

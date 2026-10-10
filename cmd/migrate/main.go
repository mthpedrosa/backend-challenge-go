package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/mthpedrosa/backend-challenge-go/db/migrations"
	"github.com/pressly/goose/v3"
)

func main() {
	flag.Usage = func() {
		fmt.Println("Usage: migrate [command]")
		fmt.Println("Commands:")
		fmt.Println("  up     - Migrate the DB to the most recent version available")
		fmt.Println("  down   - Roll back the version by 1")
		fmt.Println("  status - Print the status of all migrations")
	}
	flag.Parse()

	args := flag.Args()
	command := "up"
	if len(args) > 0 {
		command = args[0]
	}

	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		log.Fatal("DATABASE_URL environment variable is not set")
	}

	poolConfig, err := pgxpool.ParseConfig(dbURL)
	if err != nil {
		log.Fatalf("failed to parse database url: %v", err)
	}

	db := stdlib.OpenDB(*poolConfig.ConnConfig)
	defer db.Close()

	if err := db.Ping(); err != nil {
		log.Fatalf("failed to connect to database: %v", err)
	}

	goose.SetBaseFS(migrations.FS)

	if err := goose.SetDialect("postgres"); err != nil {
		log.Fatalf("failed to set dialect: %v", err)
	}

	if err := goose.RunContext(context.Background(), command, db, "."); err != nil {
		log.Fatalf("goose %v: %v", command, err)
	}
}

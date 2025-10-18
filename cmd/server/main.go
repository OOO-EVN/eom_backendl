package main

import (
	"log"
	"net/http"

	"github.com/evn/eom_backendl/config"
	"github.com/evn/eom_backendl/db"
	"github.com/evn/eom_backendl/internal/routes"
)

func main() {
	cfg := config.NewConfig()
	database := db.InitDB(cfg.DatabaseDSN)
	defer database.Close()

	redisClient := config.NewRedisClient()
	defer redisClient.Close()

	router := routes.Setup(cfg, database, redisClient)

	if err := routes.EnsureUploadDirs(); err != nil {
		log.Fatalf("Failed to create upload directories: %v", err)
	}

	go routes.AutoEndShiftsLoop(database)

	serverAddress := ":" + cfg.ServerPort
	log.Printf("🚀 Server starting on %s", serverAddress)
	log.Fatal(http.ListenAndServe(serverAddress, router))
}

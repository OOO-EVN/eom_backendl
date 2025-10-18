package routes

import (
	"log"
	"os"
	"time"

	"database/sql"

	"github.com/evn/eom_backendl/internal/handlers"
)

func EnsureUploadDirs() error {
	dirs := []string{
		"./uploads/selfies",
		"./uploads/maps",
		"./uploads/app",
	}
	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return err
		}
	}
	return nil
}

func AutoEndShiftsLoop(db *sql.DB) {
	log.Println("✅ Auto-end shifts job started")
	if count, err := handlers.AutoEndShifts(db); err != nil {
		log.Printf("❌ Startup failed: %v", err)
	} else {
		log.Printf("✅ Startup: ended %d slots", count)
	}

	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()
	for range ticker.C {
		if count, err := handlers.AutoEndShifts(db); err != nil {
			log.Printf("❌ AutoEndShifts failed: %v", err)
		} else if count > 0 {
			log.Printf("✅ AutoEndShifts: ended %d expired slots", count)
		}
	}
}

package db

import (
	"fmt"
	"log"
	"time"

	"nimble-voice-backend/config"
	"nimble-voice-backend/domain"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func ConnectPostgres(cfg *config.Config) (*gorm.DB, error) {
	dsn := fmt.Sprintf(
		"host=%s user=%s password=%s dbname=%s port=%s sslmode=%s TimeZone=%s",
		cfg.DBHost, cfg.DBUser, cfg.DBPassword, cfg.DBName, cfg.DBPort, cfg.DBSSLMode, cfg.DBTimezone,
	)

	var db *gorm.DB
	var err error

	for i := 1; i <= 5; i++ {
		db, err = gorm.Open(postgres.Open(dsn), &gorm.Config{})
		if err == nil {
			log.Println("PostgreSQL database connection established successfully")
			break
		}
		log.Printf("Attempt %d: Retrying PostgreSQL connection in 2 seconds... error: %v", i, err)
		time.Sleep(2 * time.Second)
	}

	if err != nil {
		return nil, err
	}

	// Auto migration
	_ = db.AutoMigrate(&domain.User{})
	return db, nil
}

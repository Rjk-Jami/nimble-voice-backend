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

	// Auto migration across all domain models
	if err := db.AutoMigrate(
		&domain.User{},
		&domain.Room{},
		&domain.RoomParticipant{},
		&domain.ChatMessage{},
		&domain.TopicPrompt{},
	); err != nil {
		log.Printf("AutoMigrate error: %v", err)
	}

	// Seed topic prompts if empty
	seedTopicPrompts(db)

	return db, nil
}

func seedTopicPrompts(db *gorm.DB) {
	var count int64
	db.Model(&domain.TopicPrompt{}).Count(&count)
	if count > 0 {
		return
	}

	defaultPrompts := []domain.TopicPrompt{
		{
			Category: "Daily Life & Icebreakers",
			Icon:     "coffee",
			Title:    "What is the strangest food you have ever tasted?",
			Level:    "Any",
			Tag:      "Food",
		},
		{
			Category: "Daily Life & Icebreakers",
			Icon:     "coffee",
			Title:    "If you could live in any city for a year, where would you choose?",
			Level:    "A2-B1",
			Tag:      "Travel",
		},
		{
			Category: "Tech, AI & Future",
			Icon:     "cpu",
			Title:    "Will AI change language learning forever or do we always need human conversation?",
			Level:    "B2-C1",
			Tag:      "AI",
		},
		{
			Category: "Culture & Philosophy",
			Icon:     "globe",
			Title:    "What is a tradition from your home country that foreign visitors often find surprising?",
			Level:    "B1-B2",
			Tag:      "Culture",
		},
	}

	for _, p := range defaultPrompts {
		_ = db.Create(&p)
	}
}

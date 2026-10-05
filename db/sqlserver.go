package db

import (
	"fmt"
	"log"

	"nimble-voice-backend/config"

	"gorm.io/driver/sqlserver"
	"gorm.io/gorm"
)

func ConnectSQLServer(cfg *config.Config) (*gorm.DB, error) {
	dsn := fmt.Sprintf(
		"sqlserver://%s:%s@%s:%s?database=%s",
		cfg.SQLServerUser, cfg.SQLServerPass, cfg.SQLServerHost, cfg.SQLServerPort, cfg.SQLServerDB,
	)

	db, err := gorm.Open(sqlserver.Open(dsn), &gorm.Config{})
	if err != nil {
		return nil, fmt.Errorf("failed to connect to MS SQL Server: %w", err)
	}

	log.Println("MS SQL Server connection established successfully")
	return db, nil
}

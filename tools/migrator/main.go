package main

import (
	"fmt"
	"github.com/tektoncd/results/pkg/api/server/config"
	model "github.com/tektoncd/results/pkg/api/server/db"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"log"
)

func main() {
	c := config.Get()
	dsn := fmt.Sprintf("host=%s user=%s password=%s dbname=%s port=%s sslmode=%s", c.DB_HOST, c.DB_USER, c.DB_PASSWORD, c.DB_NAME, c.DB_PORT, c.DB_SSLMODE)

	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		log.Fatalf("Failed to open the database: %v", err)
	}

	m := NewMigrator(db)
	log.Println("Starting migration..")
	if err = m.Migrate(&model.Result{}, &model.Record{}); err != nil {
		log.Fatalf("Failed to run migration: %v", err)
	}
	log.Println("Migration completed!!")
}

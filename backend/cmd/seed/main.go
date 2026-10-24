// Command seed fills a local platform database with the demo accounts from internal/demo.
package main

import (
	"log"

	"github.com/joho/godotenv"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"github.com/AthillaZaidan/datanest/backend/internal/config"
	"github.com/AthillaZaidan/datanest/backend/internal/dbconn"
	"github.com/AthillaZaidan/datanest/backend/internal/demo"
	"github.com/AthillaZaidan/datanest/backend/internal/models"
)

func main() {
	for _, f := range []string{".env", "../.env"} {
		_ = godotenv.Load(f)
	}
	cfg, err := config.Load()
	if err != nil {
		log.Fatal(err)
	}
	db, err := gorm.Open(postgres.Open(cfg.PlatformDSN), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		log.Fatal(err)
	}
	if err := models.Migrate(db); err != nil {
		log.Fatal(err)
	}
	cipher, err := dbconn.NewCipher(cfg.CredentialsKey)
	if err != nil {
		log.Fatal(err)
	}
	if err := demo.Seed(db, cipher); err != nil {
		log.Fatal(err)
	}
	for _, a := range demo.Accounts {
		role := "user"
		if a.Admin {
			role = "platform admin"
		}
		log.Printf("demo account %s (%s)", a.Username, role)
	}
}

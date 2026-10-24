// Package demo seeds fixed accounts, connections and grants for local demos. Never run it against a real platform.
package demo

import (
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/AthillaZaidan/datanest/backend/internal/auth"
	"github.com/AthillaZaidan/datanest/backend/internal/authz"
	"github.com/AthillaZaidan/datanest/backend/internal/dbconn"
	"github.com/AthillaZaidan/datanest/backend/internal/models"
)

type Account struct {
	Username string
	Password string
	Admin    bool
}

// Accounts must match frontend/src/dev/demoAccounts.ts.
var Accounts = []Account{
	{Username: "demo-admin", Password: "demoadmin123", Admin: true},
	{Username: "developer", Password: "developer123"},
	{Username: "reviewer", Password: "reviewer123"},
}

// The databases created by docker/target-init.sql.
var connections = []models.DatabaseConnection{
	{Name: "toko", Host: "localhost", Port: 15433, DBName: "toko", Username: "platform_app", SSLMode: "disable"},
	{Name: "kampus", Host: "localhost", Port: 15433, DBName: "kampus", Username: "platform_app", SSLMode: "disable"},
}

const targetPassword = "platform_app"

type group struct {
	name, description string
	members           []string
	grants            map[string]authz.Level
}

var groups = []group{
	{"Tim Developer", "Menulis dan menjalankan migration.", []string{"developer"}, map[string]authz.Level{"toko": authz.Edit, "kampus": authz.View}},
	{"Tim Reviewer", "Meninjau schema dan data.", []string{"reviewer"}, map[string]authz.Level{"toko": authz.View, "kampus": authz.View}},
}

// Seed is idempotent: it creates what is missing and resets demo passwords, connection passwords and grant levels.
// demo-admin joins no group, so it shows that a Platform Admin has no database access by itself.
func Seed(db *gorm.DB, cipher *dbconn.Cipher) error {
	return db.Transaction(func(tx *gorm.DB) error {
		var adminRole models.Role
		if err := tx.Where("name = ?", models.RolePlatformAdmin).First(&adminRole).Error; err != nil {
			return err
		}
		users := map[string]uint{}
		for _, a := range Accounts {
			hash, err := auth.HashPassword(a.Password)
			if err != nil {
				return err
			}
			u := models.User{Username: a.Username}
			if err := tx.Where(models.User{Username: a.Username}).Attrs(models.User{PasswordHash: hash}).FirstOrCreate(&u).Error; err != nil {
				return err
			}
			if err := tx.Model(&u).Update("password_hash", hash).Error; err != nil {
				return err
			}
			if a.Admin {
				if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&models.UserRole{UserID: u.ID, RoleID: adminRole.ID}).Error; err != nil {
					return err
				}
			}
			users[a.Username] = u.ID
		}

		databases := map[string]uint{}
		for _, c := range connections {
			enc, err := cipher.Encrypt(targetPassword)
			if err != nil {
				return err
			}
			if err := tx.Where(models.DatabaseConnection{Name: c.Name}).Attrs(models.DatabaseConnection{PasswordEnc: enc}).FirstOrCreate(&c).Error; err != nil {
				return err
			}
			// Re-encrypt every run so the demo survives a changed CREDENTIALS_KEY.
			if err := tx.Model(&c).Update("password_enc", enc).Error; err != nil {
				return err
			}
			databases[c.Name] = c.ID
		}

		for _, g := range groups {
			row := models.Group{Name: g.name}
			if err := tx.Where(models.Group{Name: g.name}).Attrs(models.Group{Description: g.description}).FirstOrCreate(&row).Error; err != nil {
				return err
			}
			for _, m := range g.members {
				if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&models.GroupMember{GroupID: row.ID, UserID: users[m]}).Error; err != nil {
					return err
				}
			}
			for name, level := range g.grants {
				grant := models.Grant{GroupID: row.ID, DatabaseID: databases[name], Level: int16(level)}
				err := tx.Clauses(clause.OnConflict{
					Columns:   []clause.Column{{Name: "group_id"}, {Name: "database_id"}},
					DoUpdates: clause.AssignmentColumns([]string{"level", "updated_at"}),
				}).Create(&grant).Error
				if err != nil {
					return err
				}
			}
		}
		return nil
	})
}

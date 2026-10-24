// Package testutil provides helpers for integration tests against the test-pg container.
// Run integration tests serially: go test -tags=integration -p 1 ./...
package testutil

import (
	"database/sql"
	"os"
	"testing"

	"github.com/gin-gonic/gin"
	_ "github.com/jackc/pgx/v5/stdlib"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"github.com/AthillaZaidan/datanest/backend/internal/authz"
	"github.com/AthillaZaidan/datanest/backend/internal/models"
	"github.com/AthillaZaidan/datanest/backend/internal/reqctx"
)

// Password is the plaintext password of every user created by CreateUser.
const Password = "password123"

func dsn(t *testing.T, key string) string {
	t.Helper()
	v := os.Getenv(key)
	if v == "" {
		t.Skipf("%s not set; start test-pg with docker compose and see .env.example", key)
	}
	return v
}

// PlatformDB returns a migrated, empty platform database.
func PlatformDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(postgres.Open(dsn(t, "TEST_PLATFORM_DSN")), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("open platform db: %v", err)
	}
	sqlDB, _ := db.DB()
	t.Cleanup(func() { sqlDB.Close() })

	if err := models.Migrate(db); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	err = db.Exec(`TRUNCATE users, roles, user_roles, groups, group_members,
		database_connections, grants, migrations RESTART IDENTITY CASCADE`).Error
	if err != nil {
		t.Fatalf("reset platform db: %v", err)
	}
	if err := models.Migrate(db); err != nil { // re-create built-in roles
		t.Fatalf("migrate: %v", err)
	}
	return db
}

// TargetDB returns the test target database with only an empty public schema.
func TargetDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("pgx", dsn(t, "TEST_TARGET_DSN"))
	if err != nil {
		t.Fatalf("open target db: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	_, err = db.Exec(`DO $$
		DECLARE s text;
		BEGIN
			FOR s IN SELECT nspname FROM pg_namespace
				WHERE nspname NOT IN ('pg_catalog', 'information_schema', 'pg_toast') AND nspname NOT LIKE 'pg\_%'
			LOOP
				EXECUTE format('DROP SCHEMA %I CASCADE', s);
			END LOOP;
			CREATE SCHEMA public;
		END $$`)
	if err != nil {
		t.Fatalf("reset target db: %v", err)
	}
	return db
}

func CreateUser(t *testing.T, db *gorm.DB, username string) models.User {
	t.Helper()
	hash, err := bcrypt.GenerateFromPassword([]byte(Password), bcrypt.MinCost)
	if err != nil {
		t.Fatal(err)
	}
	u := models.User{Username: username, PasswordHash: string(hash)}
	mustCreate(t, db, &u)
	return u
}

func CreateAdmin(t *testing.T, db *gorm.DB, username string) models.User {
	t.Helper()
	u := CreateUser(t, db, username)
	var role models.Role
	if err := db.Where("name = ?", models.RolePlatformAdmin).First(&role).Error; err != nil {
		t.Fatal(err)
	}
	mustCreate(t, db, &models.UserRole{UserID: u.ID, RoleID: role.ID})
	return u
}

func CreateGroup(t *testing.T, db *gorm.DB, name string) models.Group {
	t.Helper()
	g := models.Group{Name: name}
	mustCreate(t, db, &g)
	return g
}

func AddMember(t *testing.T, db *gorm.DB, groupID, userID uint) {
	t.Helper()
	mustCreate(t, db, &models.GroupMember{GroupID: groupID, UserID: userID})
}

func GrantAccess(t *testing.T, db *gorm.DB, groupID, databaseID uint, level authz.Level) {
	t.Helper()
	mustCreate(t, db, &models.Grant{GroupID: groupID, DatabaseID: databaseID, Level: int16(level)})
}

// CreateDatabaseConnection registers a connection row. The password is left empty
// until encryption exists (TODO(B)); use StaticProvider to reach the target in tests.
func CreateDatabaseConnection(t *testing.T, db *gorm.DB, name string) models.DatabaseConnection {
	t.Helper()
	c := models.DatabaseConnection{Name: name, Host: "localhost", Port: 15434, DBName: "datanest_test_target", Username: "datanest_test", PasswordEnc: []byte{}}
	mustCreate(t, db, &c)
	return c
}

// AsUser replaces RequireAuth in tests: it marks the request as made by userID.
func AsUser(userID uint) gin.HandlerFunc {
	return func(c *gin.Context) {
		reqctx.SetUserID(c, userID)
		c.Next()
	}
}

func mustCreate(t *testing.T, db *gorm.DB, v any) {
	t.Helper()
	if err := db.Create(v).Error; err != nil {
		t.Fatalf("create %T: %v", v, err)
	}
}

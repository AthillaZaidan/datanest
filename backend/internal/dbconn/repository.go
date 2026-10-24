package dbconn

import (
	"context"
	"github.com/AthillaZaidan/datanest/backend/internal/models"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type Repository struct{ DB *gorm.DB }

func NewRepository(db *gorm.DB) *Repository { return &Repository{DB: db} }
func (r *Repository) Load(ctx context.Context, id uint) (models.DatabaseConnection, error) {
	var m models.DatabaseConnection
	err := r.DB.WithContext(ctx).First(&m, id).Error
	return m, dbError(err)
}
func (r *Repository) All(ctx context.Context) ([]models.DatabaseConnection, error) {
	rows := []models.DatabaseConnection{}
	err := r.DB.WithContext(ctx).Order("id").Find(&rows).Error
	return rows, err
}
func lockRow() clause.Locking { return clause.Locking{Strength: "UPDATE"} }

package admin

import (
	"errors"
	"github.com/AthillaZaidan/datanest/backend/internal/apperr"
	"github.com/AthillaZaidan/datanest/backend/internal/models"
	"github.com/jackc/pgx/v5/pgconn"
	"gorm.io/gorm"
)

type Repository struct{ DB *gorm.DB }

func NewRepository(db *gorm.DB) *Repository { return &Repository{db} }
func mapped(err error, user bool) error {
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return apperr.ErrNotFound
	}
	var p *pgconn.PgError
	if errors.As(err, &p) {
		if p.Code == "23505" {
			if user {
				return apperr.ErrUsernameTaken
			}
			return apperr.ErrConflict
		}
		if p.Code == "23503" {
			return apperr.ErrNotFound
		}
	}
	return err
}
func exists(tx *gorm.DB, model any, id uint) error {
	var n int64
	if e := tx.Model(model).Where("id = ?", id).Count(&n).Error; e != nil {
		return e
	}
	if n == 0 {
		return apperr.ErrNotFound
	}
	return nil
}
func memberIDs(tx *gorm.DB, gid uint) ([]uint, error) {
	var ids []uint
	e := tx.Model(&models.GroupMember{}).Where("group_id = ?", gid).Pluck("user_id", &ids).Error
	return ids, e
}

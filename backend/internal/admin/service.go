package admin

import (
	"context"
	"errors"
	"github.com/AthillaZaidan/datanest/backend/internal/apperr"
	"github.com/AthillaZaidan/datanest/backend/internal/authz"
	"github.com/AthillaZaidan/datanest/backend/internal/models"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"strings"
	"unicode/utf8"
)

type SessionRevoker interface {
	MarkNeedsRotation(uint)
	RevokeAllForUser(uint)
}
type Service struct {
	repo     *Repository
	sessions SessionRevoker
}

func New(db *gorm.DB, s SessionRevoker) *Service { return &Service{NewRepository(db), s} }

func (s *Service) require(ctx context.Context, actor uint) error {
	var n int64
	e := s.repo.DB.WithContext(ctx).Table("user_roles ur").Joins("JOIN roles r ON r.id = ur.role_id").Where("ur.user_id = ? AND r.name = ?", actor, models.RolePlatformAdmin).Count(&n).Error
	if e != nil {
		return e
	}
	if n == 0 {
		return apperr.ErrForbidden
	}
	return nil
}
func validName(n string) error {
	if strings.TrimSpace(n) == "" || utf8.RuneCountInString(n) > 64 {
		return apperr.ErrValidation.WithMessage("Nama wajib diisi dan maksimal 64 karakter.")
	}
	return nil
}
func hash(p string) (string, error) {
	if utf8.RuneCountInString(p) < 8 || len(p) > 72 {
		return "", apperr.ErrValidation.WithDetails(map[string]string{"password": "Minimal 8 karakter dan maksimal 72 byte."})
	}
	b, e := bcrypt.GenerateFromPassword([]byte(p), bcrypt.DefaultCost)
	return string(b), e
}

type UserView struct {
	ID       uint   `json:"id"`
	Username string `json:"username"`
	IsAdmin  bool   `json:"isAdmin"`
}

func userQuery(db *gorm.DB) *gorm.DB {
	return db.Table("users u").Select("u.id, u.username, EXISTS (SELECT 1 FROM user_roles ur JOIN roles r ON r.id = ur.role_id WHERE ur.user_id = u.id AND r.name = 'platform_admin') AS is_admin")
}
func (s *Service) Users(ctx context.Context, a uint) ([]UserView, error) {
	if e := s.require(ctx, a); e != nil {
		return nil, e
	}
	out := []UserView{}
	e := userQuery(s.repo.DB.WithContext(ctx)).Order("u.id").Scan(&out).Error
	return out, e
}
func (s *Service) User(ctx context.Context, a, id uint) (UserView, error) {
	if e := s.require(ctx, a); e != nil {
		return UserView{}, e
	}
	var out UserView
	res := userQuery(s.repo.DB.WithContext(ctx)).Where("u.id = ?", id).Scan(&out)
	if res.Error != nil {
		return out, res.Error
	}
	if res.RowsAffected == 0 {
		return out, apperr.ErrNotFound
	}
	return out, nil
}
func (s *Service) CreateUser(ctx context.Context, a uint, name, password string) (UserView, error) {
	if e := s.require(ctx, a); e != nil {
		return UserView{}, e
	}
	if e := validName(name); e != nil {
		return UserView{}, e
	}
	h, e := hash(password)
	if e != nil {
		return UserView{}, e
	}
	u := models.User{Username: name, PasswordHash: h}
	if e = s.repo.DB.WithContext(ctx).Create(&u).Error; e != nil {
		return UserView{}, mapped(e, true)
	}
	return UserView{ID: u.ID, Username: u.Username}, nil
}

type UserPatch struct {
	Username *string `json:"username"`
	Password *string `json:"password"`
}

func (s *Service) UpdateUser(ctx context.Context, a, id uint, p UserPatch) (UserView, error) {
	if e := s.require(ctx, a); e != nil {
		return UserView{}, e
	}
	changes := map[string]any{}
	if p.Username != nil {
		if e := validName(*p.Username); e != nil {
			return UserView{}, e
		}
		changes["username"] = *p.Username
	}
	if p.Password != nil {
		h, e := hash(*p.Password)
		if e != nil {
			return UserView{}, e
		}
		changes["password_hash"] = h
	}
	if len(changes) == 0 {
		return UserView{}, apperr.ErrValidation
	}
	e := s.repo.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var u models.User
		if e := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&u, id).Error; e != nil {
			return mapped(e, true)
		}
		return mapped(tx.Model(&u).Updates(changes).Error, true)
	})
	if e != nil {
		return UserView{}, e
	}
	return s.User(ctx, a, id)
}
func (s *Service) DeleteUser(ctx context.Context, a, id uint) error {
	if e := s.require(ctx, a); e != nil {
		return e
	}
	e := s.repo.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if e := keepAnAdmin(tx, id); e != nil {
			return e
		}
		res := tx.Delete(&models.User{}, id)
		if res.Error != nil {
			return mapped(res.Error, false)
		}
		if res.RowsAffected == 0 {
			return apperr.ErrNotFound
		}
		return nil
	})
	if e != nil {
		return e
	}
	s.sessions.RevokeAllForUser(id)
	return nil
}
func (s *Service) Role(ctx context.Context, a, id uint, role string, add bool) error {
	if e := s.require(ctx, a); e != nil {
		return e
	}
	if role != models.RolePlatformAdmin {
		return apperr.ErrValidation.WithMessage("Role tidak dikenal.")
	}
	changed := false
	e := s.repo.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if e := exists(tx, &models.User{}, id); e != nil {
			return e
		}
		var r models.Role
		if e := tx.Where("name = ?", role).First(&r).Error; e != nil {
			return e
		}
		ur := models.UserRole{UserID: id, RoleID: r.ID}
		if add {
			res := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&ur)
			changed = res.RowsAffected > 0
			return res.Error
		}
		if e := keepAnAdmin(tx, id); e != nil {
			return e
		}
		return tx.Where("user_id = ? AND role_id = ?", id, r.ID).Delete(&models.UserRole{}).Error
	})
	if e == nil {
		if add && changed {
			s.sessions.MarkNeedsRotation(id)
		} else if !add {
			s.sessions.RevokeAllForUser(id)
		}
	}
	return mapped(e, false)
}

type GroupView struct {
	ID          uint   `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
}

func gv(g models.Group) GroupView { return GroupView{g.ID, g.Name, g.Description} }
func (s *Service) Groups(ctx context.Context, a uint) ([]GroupView, error) {
	if e := s.require(ctx, a); e != nil {
		return nil, e
	}
	var gs []models.Group
	if e := s.repo.DB.WithContext(ctx).Order("id").Find(&gs).Error; e != nil {
		return nil, e
	}
	out := make([]GroupView, 0, len(gs))
	for _, g := range gs {
		out = append(out, gv(g))
	}
	return out, nil
}
func (s *Service) Group(ctx context.Context, a, id uint) (GroupView, error) {
	if e := s.require(ctx, a); e != nil {
		return GroupView{}, e
	}
	var g models.Group
	e := s.repo.DB.WithContext(ctx).First(&g, id).Error
	return gv(g), mapped(e, false)
}
func (s *Service) CreateGroup(ctx context.Context, a uint, name, description string) (GroupView, error) {
	if e := s.require(ctx, a); e != nil {
		return GroupView{}, e
	}
	if e := validName(name); e != nil {
		return GroupView{}, e
	}
	g := models.Group{Name: name, Description: description}
	e := s.repo.DB.WithContext(ctx).Create(&g).Error
	return gv(g), mapped(e, false)
}

type GroupPatch struct {
	Name        *string `json:"name"`
	Description *string `json:"description"`
}

func (s *Service) UpdateGroup(ctx context.Context, a, id uint, p GroupPatch) (GroupView, error) {
	if e := s.require(ctx, a); e != nil {
		return GroupView{}, e
	}
	changes := map[string]any{}
	if p.Name != nil {
		if e := validName(*p.Name); e != nil {
			return GroupView{}, e
		}
		changes["name"] = *p.Name
	}
	if p.Description != nil {
		changes["description"] = *p.Description
	}
	if len(changes) == 0 {
		return GroupView{}, apperr.ErrValidation
	}
	res := s.repo.DB.WithContext(ctx).Model(&models.Group{}).Where("id = ?", id).Updates(changes)
	if res.Error != nil {
		return GroupView{}, mapped(res.Error, false)
	}
	if res.RowsAffected == 0 {
		return GroupView{}, apperr.ErrNotFound
	}
	return s.Group(ctx, a, id)
}
func (s *Service) DeleteGroup(ctx context.Context, a, id uint) error {
	if e := s.require(ctx, a); e != nil {
		return e
	}
	res := s.repo.DB.WithContext(ctx).Delete(&models.Group{}, id)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return apperr.ErrNotFound
	}
	return nil
}
func (s *Service) Members(ctx context.Context, a, gid uint) ([]UserView, error) {
	if e := s.require(ctx, a); e != nil {
		return nil, e
	}
	if e := exists(s.repo.DB.WithContext(ctx), &models.Group{}, gid); e != nil {
		return nil, e
	}
	out := []UserView{}
	e := userQuery(s.repo.DB.WithContext(ctx)).Joins("JOIN group_members gm ON gm.user_id = u.id").Where("gm.group_id = ?", gid).Order("u.id").Scan(&out).Error
	return out, e
}
func (s *Service) Member(ctx context.Context, a, gid, uid uint, add bool) error {
	if e := s.require(ctx, a); e != nil {
		return e
	}
	changed := false
	e := s.repo.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if e := exists(tx, &models.Group{}, gid); e != nil {
			return e
		}
		if e := exists(tx, &models.User{}, uid); e != nil {
			return e
		}
		if add {
			m := models.GroupMember{GroupID: gid, UserID: uid}
			res := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&m)
			changed = res.RowsAffected > 0
			return res.Error
		}
		return tx.Where("group_id = ? AND user_id = ?", gid, uid).Delete(&models.GroupMember{}).Error
	})
	if e == nil && changed {
		s.sessions.MarkNeedsRotation(uid)
	}
	return mapped(e, false)
}

type GrantView struct {
	GroupID    uint   `json:"groupId"`
	DatabaseID uint   `json:"databaseId"`
	Level      string `json:"level"`
}

func (s *Service) Grants(ctx context.Context, a, gid uint) ([]GrantView, error) {
	if e := s.require(ctx, a); e != nil {
		return nil, e
	}
	if e := exists(s.repo.DB.WithContext(ctx), &models.Group{}, gid); e != nil {
		return nil, e
	}
	var rows []models.Grant
	if e := s.repo.DB.WithContext(ctx).Where("group_id = ?", gid).Order("database_id").Find(&rows).Error; e != nil {
		return nil, e
	}
	out := make([]GrantView, 0, len(rows))
	for _, g := range rows {
		out = append(out, GrantView{gid, g.DatabaseID, authz.Level(g.Level).String()})
	}
	return out, nil
}
func (s *Service) Grant(ctx context.Context, a, gid, did uint, level string, add bool) error {
	if e := s.require(ctx, a); e != nil {
		return e
	}
	var l authz.Level
	var e error
	if add {
		l, e = authz.ParseLevel(level)
		if e != nil {
			return apperr.ErrValidation.WithDetails(map[string]string{"level": "Gunakan VIEW atau EDIT."})
		}
	}
	var users []uint
	raised := false
	e = s.repo.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// Lock the group even when no grant exists, so concurrent upserts compare a stable old level.
		var group models.Group
		if e := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&group, gid).Error; e != nil {
			return mapped(e, false)
		}
		if e := exists(tx, &models.DatabaseConnection{}, did); e != nil {
			return e
		}
		if !add {
			return tx.Where("group_id = ? AND database_id = ?", gid, did).Delete(&models.Grant{}).Error
		}
		var old models.Grant
		e := tx.Where("group_id = ? AND database_id = ?", gid, did).First(&old).Error
		if e != nil && !errors.Is(e, gorm.ErrRecordNotFound) {
			return e
		}
		raised = int16(l) > old.Level
		g := models.Grant{GroupID: gid, DatabaseID: did, Level: int16(l)}
		if e := tx.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "group_id"}, {Name: "database_id"}}, DoUpdates: clause.AssignmentColumns([]string{"level", "updated_at"})}).Create(&g).Error; e != nil {
			return e
		}
		if raised {
			users, e = memberIDs(tx, gid)
			return e
		}
		return nil
	})
	if e == nil && raised {
		for _, u := range users {
			s.sessions.MarkNeedsRotation(u)
		}
	}
	return mapped(e, false)
}

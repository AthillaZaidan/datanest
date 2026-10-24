package dbconn

import (
	"context"
	"errors"
	"github.com/AthillaZaidan/datanest/backend/internal/apperr"
	"github.com/AthillaZaidan/datanest/backend/internal/authz"
	"github.com/AthillaZaidan/datanest/backend/internal/models"
	"github.com/jackc/pgx/v5/pgconn"
	"gorm.io/gorm"
	"strings"
	"time"
)

type Input struct {
	Name     string `json:"name"`
	Host     string `json:"host"`
	Port     int    `json:"port"`
	DBName   string `json:"dbName"`
	Username string `json:"username"`
	Password string `json:"password"`
	SSLMode  string `json:"sslMode"`
}
type Patch struct {
	Name     *string `json:"name"`
	Host     *string `json:"host"`
	Port     *int    `json:"port"`
	DBName   *string `json:"dbName"`
	Username *string `json:"username"`
	Password *string `json:"password"`
	SSLMode  *string `json:"sslMode"`
}
type View struct {
	ID             uint   `json:"id"`
	Name           string `json:"name"`
	Host           string `json:"host"`
	Port           int    `json:"port"`
	DBName         string `json:"dbName"`
	Username       string `json:"username"`
	SSLMode        string `json:"sslMode"`
	HasPassword    bool   `json:"hasPassword"`
	MyLevel        string `json:"myLevel,omitempty"`
	CurrentVersion int    `json:"currentVersion"`
}

func view(m models.DatabaseConnection) View {
	return View{ID: m.ID, Name: m.Name, Host: m.Host, Port: m.Port, DBName: m.DBName, Username: m.Username, SSLMode: m.SSLMode, HasPassword: len(m.PasswordEnc) > 0}
}

type Service struct {
	db      *gorm.DB
	cipher  *Cipher
	pools   *Pools
	az      *authz.Service
	timeout time.Duration
}

func New(db *gorm.DB, c *Cipher, p *Pools, az *authz.Service, t time.Duration) *Service {
	return &Service{db, c, p, az, t}
}
func validate(i *Input) error {
	if i.SSLMode == "" {
		i.SSLMode = "disable"
	}
	if strings.TrimSpace(i.Name) == "" || len(i.Name) > 64 || strings.TrimSpace(i.Host) == "" || strings.TrimSpace(i.DBName) == "" || strings.TrimSpace(i.Username) == "" || i.Port < 1 || i.Port > 65535 {
		return apperr.ErrValidation.WithMessage("Nama, host, port (1–65535), database, dan username wajib valid.")
	}
	switch i.SSLMode {
	case "disable", "allow", "prefer", "require", "verify-ca", "verify-full":
	default:
		return apperr.ErrValidation.WithMessage("sslMode tidak valid.")
	}
	return nil
}
func dbError(e error) error {
	if errors.Is(e, gorm.ErrRecordNotFound) {
		return apperr.ErrNotFound
	}
	var p *pgconn.PgError
	if errors.As(e, &p) && p.Code == "23505" {
		return apperr.ErrConflict
	}
	return e
}
func (s *Service) load(ctx context.Context, id uint) (models.DatabaseConnection, error) {
	return NewRepository(s.db).Load(ctx, id)
}
func (s *Service) List(ctx context.Context) ([]View, error) {
	ms, e := NewRepository(s.db).All(ctx)
	if e != nil {
		return nil, e
	}
	out := make([]View, 0, len(ms))
	for _, m := range ms {
		out = append(out, view(m))
	}
	return out, nil
}
func (s *Service) Get(ctx context.Context, id uint) (View, error) {
	m, e := s.load(ctx, id)
	return view(m), e
}
func (s *Service) Create(ctx context.Context, i Input) (View, error) {
	if e := validate(&i); e != nil {
		return View{}, e
	}
	enc, e := s.cipher.Encrypt(i.Password)
	if e != nil {
		return View{}, e
	}
	m := models.DatabaseConnection{Name: i.Name, Host: i.Host, Port: i.Port, DBName: i.DBName, Username: i.Username, PasswordEnc: enc, SSLMode: i.SSLMode}
	e = s.db.WithContext(ctx).Create(&m).Error
	return view(m), dbError(e)
}
func (s *Service) Update(ctx context.Context, id uint, p Patch) (View, error) {
	var out View
	e := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		m := models.DatabaseConnection{}
		if e := tx.Clauses(lockRow()).First(&m, id).Error; e != nil {
			return dbError(e)
		}
		i := Input{Name: m.Name, Host: m.Host, Port: m.Port, DBName: m.DBName, Username: m.Username, SSLMode: m.SSLMode}
		if p.Name != nil {
			i.Name = *p.Name
		}
		if p.Host != nil {
			i.Host = *p.Host
		}
		if p.Port != nil {
			i.Port = *p.Port
		}
		if p.DBName != nil {
			i.DBName = *p.DBName
		}
		if p.Username != nil {
			i.Username = *p.Username
		}
		if p.SSLMode != nil {
			i.SSLMode = *p.SSLMode
		}
		if e := validate(&i); e != nil {
			return e
		}
		changes := map[string]any{"name": i.Name, "host": i.Host, "port": i.Port, "db_name": i.DBName, "username": i.Username, "ssl_mode": i.SSLMode}
		if p.Password != nil && *p.Password != "" {
			enc, e := s.cipher.Encrypt(*p.Password)
			if e != nil {
				return e
			}
			changes["password_enc"] = enc
		}
		if e := tx.Model(&m).Updates(changes).Error; e != nil {
			return dbError(e)
		}
		out = view(m)
		return nil
	})
	if e == nil {
		s.pools.Invalidate(id)
	}
	return out, e
}
func (s *Service) Delete(ctx context.Context, id uint) error {
	res := s.db.WithContext(ctx).Delete(&models.DatabaseConnection{}, id)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return apperr.ErrNotFound
	}
	s.pools.Invalidate(id)
	return nil
}

type TestResult struct {
	OK          bool   `json:"ok"`
	IsSuperuser bool   `json:"isSuperuser"`
	Warning     string `json:"warning,omitempty"`
}

func (s *Service) Test(ctx context.Context, i Input) (TestResult, error) {
	if e := validate(&i); e != nil {
		return TestResult{}, e
	}
	m := models.DatabaseConnection{Host: i.Host, Port: i.Port, DBName: i.DBName, Username: i.Username, SSLMode: i.SSLMode}
	pool, e := openTarget(m, i.Password, s.timeout)
	if e != nil {
		return TestResult{}, e
	}
	defer pool.Close()
	t, cancel := context.WithTimeout(ctx, s.timeout)
	defer cancel()
	var one int
	if e = pool.QueryRowContext(t, "SELECT 1").Scan(&one); e != nil {
		return TestResult{}, ConnectionError(t, e)
	}
	out := TestResult{OK: true}
	if e = pool.QueryRowContext(t, "SELECT rolsuper FROM pg_roles WHERE rolname = current_user").Scan(&out.IsSuperuser); e != nil {
		return TestResult{}, ConnectionError(t, e)
	}
	if out.IsSuperuser {
		out.Warning = "Role koneksi adalah superuser. Gunakan role non-superuser dengan hak minimum yang diperlukan."
	}
	return out, nil
}
func (s *Service) TestSaved(ctx context.Context, id uint) (TestResult, error) {
	m, e := s.load(ctx, id)
	if e != nil {
		return TestResult{}, e
	}
	p, e := s.cipher.Decrypt(m.PasswordEnc)
	if e != nil {
		return TestResult{}, apperr.ErrInternal
	}
	return s.Test(ctx, Input{Name: m.Name, Host: m.Host, Port: m.Port, DBName: m.DBName, Username: m.Username, Password: p, SSLMode: m.SSLMode})
}
func (s *Service) ListForUser(ctx context.Context, uid uint) ([]View, error) {
	var rows []struct {
		models.DatabaseConnection
		Level int16
	}
	e := s.db.WithContext(ctx).Table("database_connections d").Select("d.*, MAX(g.level) AS level").Joins("JOIN grants g ON g.database_id = d.id").Joins("JOIN group_members gm ON gm.group_id = g.group_id").Where("gm.user_id = ?", uid).Group("d.id").Order("d.id").Scan(&rows).Error
	if e != nil {
		return nil, e
	}
	out := make([]View, 0, len(rows))
	for _, m := range rows {
		v := view(m.DatabaseConnection)
		v.MyLevel = authz.Level(m.Level).String()
		out = append(out, v)
	}
	return out, nil
}
func (s *Service) GetForUser(ctx context.Context, uid, id uint) (View, error) {
	m, e := s.load(ctx, id)
	if e != nil {
		return View{}, e
	}
	ok, e := s.az.Can(ctx, uid, id, authz.ViewDatabase)
	if e != nil {
		return View{}, e
	}
	if !ok {
		return View{}, apperr.ErrForbidden
	}
	level, e := s.az.EffectiveLevel(ctx, uid, id)
	if e != nil {
		return View{}, e
	}
	v := view(m)
	v.MyLevel = level.String()
	e = s.db.WithContext(ctx).Model(&models.Migration{}).Select("COALESCE(MAX(version),0)").Where("database_id = ? AND status = ?", id, models.MigrationSuccess).Scan(&v.CurrentVersion).Error
	return v, e
}

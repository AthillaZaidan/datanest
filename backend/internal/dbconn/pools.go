package dbconn

import (
	"context"
	"database/sql"
	"errors"
	"github.com/AthillaZaidan/datanest/backend/internal/apperr"
	"github.com/AthillaZaidan/datanest/backend/internal/models"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/stdlib"
	"gorm.io/gorm"
	"sync"
	"time"
)

type Pools struct {
	mu      sync.Mutex
	db      *gorm.DB
	cipher  *Cipher
	timeout time.Duration
	entries map[uint]*sql.DB
	closed  bool
}

func NewPools(db *gorm.DB, c *Cipher, timeout time.Duration) *Pools {
	return &Pools{db: db, cipher: c, timeout: timeout, entries: map[uint]*sql.DB{}}
}

// openTarget starts from an empty parsed config, never a DSN containing credentials.
// Clear fallback hosts: every attempted connection must use the configured target.
func openTarget(m models.DatabaseConnection, password string, timeout time.Duration) (*sql.DB, error) {
	cfg, e := pgx.ParseConfig("postgres://localhost/postgres?sslmode=" + m.SSLMode)
	if e != nil {
		return nil, apperr.ErrValidation
	}
	cfg.Host = m.Host
	cfg.Port = uint16(m.Port)
	cfg.Database = m.DBName
	cfg.User = m.Username
	cfg.Password = password
	cfg.ConnectTimeout = timeout
	for _, f := range cfg.Fallbacks {
		f.Host = m.Host
		f.Port = uint16(m.Port)
	}
	if cfg.TLSConfig != nil {
		cfg.TLSConfig.ServerName = m.Host
	}
	for _, f := range cfg.Fallbacks {
		if f.TLSConfig != nil {
			f.TLSConfig.ServerName = m.Host
		}
	}
	cfg.RuntimeParams = map[string]string{}
	pool := stdlib.OpenDB(*cfg)
	pool.SetMaxOpenConns(5)
	pool.SetMaxIdleConns(5)
	pool.SetConnMaxIdleTime(5 * time.Minute)
	return pool, nil
}
func (p *Pools) Get(ctx context.Context, id uint) (*sql.DB, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed {
		return nil, apperr.ErrTargetDBUnavailable
	}
	if pool := p.entries[id]; pool != nil {
		return pool, nil
	}
	var m models.DatabaseConnection
	if e := p.db.WithContext(ctx).First(&m, id).Error; e != nil {
		if errors.Is(e, gorm.ErrRecordNotFound) {
			return nil, apperr.ErrNotFound
		}
		return nil, e
	}
	password, e := p.cipher.Decrypt(m.PasswordEnc)
	if e != nil {
		return nil, apperr.ErrInternal
	}
	pool, e := openTarget(m, password, p.timeout)
	if e != nil {
		return nil, e
	}
	t, cancel := context.WithTimeout(ctx, p.timeout)
	defer cancel()
	if e = pool.PingContext(t); e != nil {
		pool.Close()
		return nil, ConnectionError(t, e)
	}
	p.entries[id] = pool
	return pool, nil
}
func (p *Pools) Invalidate(id uint) {
	p.mu.Lock()
	pool := p.entries[id]
	delete(p.entries, id)
	p.mu.Unlock()
	if pool != nil {
		_ = pool.Close()
	}
}
func (p *Pools) Close() {
	p.mu.Lock()
	p.closed = true
	entries := p.entries
	p.entries = map[uint]*sql.DB{}
	p.mu.Unlock()
	for _, pool := range entries {
		_ = pool.Close()
	}
}

// Driver messages may contain credentials. Only expose fixed strings.
func ConnectionError(ctx context.Context, err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(ctx.Err(), context.DeadlineExceeded) || errors.Is(err, context.DeadlineExceeded) || pgconn.Timeout(err) {
		return apperr.ErrTargetDBTimeout
	}
	return apperr.ErrTargetDBUnavailable
}

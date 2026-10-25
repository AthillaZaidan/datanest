// Package auth implements password authentication and HTTP session handling.
package auth

import (
	"crypto/subtle"
	"errors"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"

	"github.com/AthillaZaidan/datanest/backend/internal/apperr"
	"github.com/AthillaZaidan/datanest/backend/internal/models"
	"github.com/AthillaZaidan/datanest/backend/internal/reqctx"
	"github.com/AthillaZaidan/datanest/backend/internal/session"
)

const CookieName = "sid"

var dummyHash, _ = bcrypt.GenerateFromPassword([]byte("datanest-invalid-password"), bcrypt.DefaultCost)

type Service struct {
	DB           *gorm.DB
	Sessions     *session.Store
	CookieSecure bool
}
type UserResponse struct {
	ID       uint   `json:"id"`
	Username string `json:"username"`
	IsAdmin  bool   `json:"isAdmin"`
}
type loginInput struct {
	Username string `json:"username"`
	Password string `json:"password"`
}
type loginResponse struct {
	User      UserResponse `json:"user"`
	CSRFToken string       `json:"csrfToken"`
}

func HashPassword(password string) (string, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	return string(hash), err
}

func (s *Service) Login(c *gin.Context) {
	var input loginInput
	if err := c.ShouldBindJSON(&input); err != nil || strings.TrimSpace(input.Username) == "" || input.Password == "" {
		respond(c, apperr.ErrValidation.WithDetails(map[string]string{"username": "username dan password wajib diisi"}))
		return
	}
	u, err := s.Verify(input.Username, input.Password)
	if err != nil {
		respond(c, err)
		return
	}
	csrf, admin, err := s.StartSession(c, u.ID)
	if err != nil {
		respond(c, apperr.ErrInternal)
		return
	}
	c.JSON(http.StatusOK, loginResponse{User: UserResponse{ID: u.ID, Username: u.Username, IsAdmin: admin}, CSRFToken: csrf})
}

// Verify checks a username and password. Unknown users still pay for a bcrypt comparison so timing does not reveal them.
func (s *Service) Verify(username, password string) (models.User, error) {
	var u models.User
	err := s.DB.Where("username = ?", username).First(&u).Error
	hash := []byte(u.PasswordHash)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		hash = dummyHash
	}
	if err != nil || bcrypt.CompareHashAndPassword(hash, []byte(password)) != nil {
		return models.User{}, apperr.ErrInvalidCredentials
	}
	return u, nil
}

// StartSession always creates a new session (no fixation) and sets its cookie.
func (s *Service) StartSession(c *gin.Context, userID uint) (csrf string, admin bool, err error) {
	raw, sess, err := s.Sessions.Create(userID)
	if err != nil {
		return "", false, err
	}
	if admin, err = s.IsAdmin(userID); err != nil {
		return "", false, err
	}
	s.setCookie(c, raw)
	return sess.CSRFToken, admin, nil
}

func (s *Service) Logout(c *gin.Context) {
	s.EndSession(c)
	c.Status(http.StatusNoContent)
}

// EndSession revokes the current session, including identifiers still in grace, and clears the cookie.
func (s *Service) EndSession(c *gin.Context) {
	if raw, err := c.Cookie(CookieName); err == nil {
		s.Sessions.Revoke(raw)
	}
	http.SetCookie(c.Writer, &http.Cookie{Name: CookieName, Value: "", Path: "/", HttpOnly: true, Secure: s.CookieSecure, SameSite: http.SameSiteStrictMode, MaxAge: -1})
}
func (s *Service) Me(c *gin.Context) {
	id, ok := reqctx.UserID(c)
	if !ok {
		respond(c, apperr.ErrUnauthenticated)
		return
	}
	var u models.User
	if err := s.DB.First(&u, id).Error; err != nil {
		respond(c, apperr.ErrUnauthenticated)
		return
	}
	admin, err := s.IsAdmin(id)
	if err != nil {
		respond(c, apperr.ErrInternal)
		return
	}
	token, _ := c.Get("datanest.csrf")
	c.JSON(http.StatusOK, loginResponse{User: UserResponse{ID: u.ID, Username: u.Username, IsAdmin: admin}, CSRFToken: token.(string)})
}

func (s *Service) RequireAuth(c *gin.Context) {
	sess, ok := s.Authenticate(c)
	if !ok {
		respond(c, apperr.ErrUnauthenticated)
		return
	}
	if stateChanging(c.Request.Method) && !constantEqual(c.GetHeader("X-CSRF-Token"), sess.CSRFToken) {
		respond(c, apperr.ErrCSRFInvalid)
		return
	}
	c.Next()
}

// Authenticate validates the session cookie, sends a rotated identifier when due, and stores the user in the context.
func (s *Service) Authenticate(c *gin.Context) (*session.Session, bool) {
	raw, err := c.Cookie(CookieName)
	if err != nil {
		return nil, false
	}
	sess, replacement, err := s.Sessions.Validate(raw)
	if err != nil {
		return nil, false
	}
	if replacement != "" {
		s.setCookie(c, replacement)
	}
	reqctx.SetUserID(c, sess.UserID)
	c.Set("datanest.csrf", sess.CSRFToken)
	return sess, true
}

// CSRFMatches compares a submitted token with the session's in constant time.
func CSRFMatches(token string, sess *session.Session) bool {
	return constantEqual(token, sess.CSRFToken)
}

func (s *Service) RequireAdmin(c *gin.Context) {
	id, ok := reqctx.UserID(c)
	if !ok {
		respond(c, apperr.ErrUnauthenticated)
		return
	}
	admin, err := s.IsAdmin(id)
	if err != nil {
		respond(c, apperr.ErrInternal)
		return
	}
	if !admin {
		respond(c, apperr.ErrForbidden)
		return
	}
	c.Next()
}

// ValidateStateRequest protects both login and authenticated state changes.
func ValidateStateRequest(allowedOrigin string) gin.HandlerFunc {
	return func(c *gin.Context) {
		if !stateChanging(c.Request.Method) {
			c.Next()
			return
		}
		if origin := c.GetHeader("Origin"); origin != "" && origin != allowedOrigin {
			respond(c, apperr.ErrOriginNotAllowed)
			return
		}
		ct := strings.ToLower(c.GetHeader("Content-Type"))
		form := !strings.HasPrefix(c.Request.URL.Path, "/api/") && strings.HasPrefix(ct, "application/x-www-form-urlencoded")
		if !form && !strings.HasPrefix(ct, "application/json") {
			respond(c, apperr.ErrUnsupportedMediaType)
			return
		}
		c.Next()
	}
}
func (s *Service) IsAdmin(userID uint) (bool, error) {
	var n int64
	err := s.DB.Model(&models.UserRole{}).Joins("JOIN roles ON roles.id = user_roles.role_id").Where("user_roles.user_id = ? AND roles.name = ?", userID, models.RolePlatformAdmin).Count(&n).Error
	return n > 0, err
}
func (s *Service) setCookie(c *gin.Context, raw string) {
	http.SetCookie(c.Writer, &http.Cookie{Name: CookieName, Value: raw, Path: "/", HttpOnly: true, Secure: s.CookieSecure, SameSite: http.SameSiteStrictMode})
}
func stateChanging(method string) bool {
	return method == http.MethodPost || method == http.MethodPut || method == http.MethodPatch || method == http.MethodDelete
}
func constantEqual(a, b string) bool {
	if len(a) != len(b) {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1
}
func respond(c *gin.Context, err error) {
	status, body := apperr.Response(err)
	c.AbortWithStatusJSON(status, body)
}

// SeedAdmin creates exactly one initial administrator. Existing admins are never reset.
func SeedAdmin(db *gorm.DB, username, password string) error {
	var count int64
	if err := db.Model(&models.UserRole{}).Joins("JOIN roles ON roles.id = user_roles.role_id").Where("roles.name = ?", models.RolePlatformAdmin).Count(&count).Error; err != nil {
		return err
	}
	if count > 0 {
		return nil
	}
	if strings.TrimSpace(username) == "" || password == "" {
		return errors.New("ADMIN_USERNAME and ADMIN_PASSWORD are required when no platform admin exists")
	}
	hash, err := HashPassword(password)
	if err != nil {
		return err
	}
	return db.Transaction(func(tx *gorm.DB) error {
		var role models.Role
		if err := tx.Where("name = ?", models.RolePlatformAdmin).First(&role).Error; err != nil {
			return err
		}
		u := models.User{Username: username, PasswordHash: hash}
		if err := tx.Create(&u).Error; err != nil {
			return err
		}
		return tx.Create(&models.UserRole{UserID: u.ID, RoleID: role.ID}).Error
	})
}

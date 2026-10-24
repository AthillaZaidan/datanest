package reqctx

import (
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestUserID(t *testing.T) {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())

	if _, ok := UserID(c); ok {
		t.Fatal("UserID should be absent before SetUserID")
	}
	SetUserID(c, 42)
	if id, ok := UserID(c); !ok || id != 42 {
		t.Fatalf("UserID = %d, %v; want 42, true", id, ok)
	}
}

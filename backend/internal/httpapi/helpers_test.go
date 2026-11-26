package httpapi

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestPathIDKeepsIDsWithinTheRangeOfUint(t *testing.T) {
	gin.SetMode(gin.TestMode)
	largest := strconv.FormatUint(uint64(^uint(0)), 10)
	cases := []struct {
		raw    string
		wantID uint
		wantOK bool
	}{
		{"42", 42, true},
		{largest, ^uint(0), true},
		{largest + "0", 0, false},
		{"0", 0, false},
		{"-1", 0, false},
		{"abc", 0, false},
	}
	for _, c := range cases {
		w := httptest.NewRecorder()
		ctx, _ := gin.CreateTestContext(w)
		ctx.Params = gin.Params{{Key: "id", Value: c.raw}}
		id, ok := pathID(ctx, "id")
		if id != c.wantID || ok != c.wantOK {
			t.Errorf("pathID(%q) = %d, %v; want %d, %v", c.raw, id, ok, c.wantID, c.wantOK)
		}
		if !ok && w.Code != http.StatusNotFound {
			t.Errorf("pathID(%q) wrote %d, want 404", c.raw, w.Code)
		}
	}
}

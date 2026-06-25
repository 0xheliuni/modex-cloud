package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/modex/modex-cloud/constant"
	"github.com/modex/modex-cloud/model"

	"github.com/gin-contrib/sessions"
	"github.com/gin-contrib/sessions/cookie"
	"github.com/gin-gonic/gin"
)

// newRouter builds a gin engine with a cookie session store and the three
// protected route groups, plus a seeded user per role for access-token tests.
func newRouter(t *testing.T) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	cleanup, err := model.InitForTest()
	if err != nil {
		t.Fatalf("InitForTest: %v", err)
	}
	t.Cleanup(cleanup)

	r := gin.New()
	store := cookie.NewStore([]byte("test-session-secret-32-bytes-xxxx"))
	r.Use(sessions.Sessions("modex_session", store))

	r.GET("/supplier", SupplierAuth(), func(c *gin.Context) { c.String(http.StatusOK, "supplier-ok") })
	r.GET("/admin", AdminAuth(), func(c *gin.Context) { c.String(http.StatusOK, "admin-ok") })
	r.GET("/root", RootAuth(), func(c *gin.Context) { c.String(http.StatusOK, "root-ok") })
	return r
}

// seedUserWithToken creates a user of the given role with a known access token.
func seedUserWithToken(t *testing.T, role int, token string) *model.User {
	t.Helper()
	tok := token
	u := &model.User{Username: "u" + token[:4], Role: role, Status: constant.StatusEnabled, AccessToken: &tok}
	if err := u.Create("password123"); err != nil {
		t.Fatalf("seed user: %v", err)
	}
	return u
}

func req(r *gin.Engine, path, token string) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	rq, _ := http.NewRequest(http.MethodGet, path, nil)
	if token != "" {
		rq.Header.Set("Authorization", "Bearer "+token)
	}
	r.ServeHTTP(w, rq)
	return w
}

func TestRBAC_AccessTokenEnforcesRole(t *testing.T) {
	r := newRouter(t)
	seedUserWithToken(t, constant.RoleSupplier, "suppliertoken0001")
	seedUserWithToken(t, constant.RoleAdmin, "admintoken00000001")

	tests := []struct {
		name, path, token string
		wantStatus        int
	}{
		{"supplier hits supplier", "/supplier", "suppliertoken0001", http.StatusOK},
		{"supplier blocked from admin", "/admin", "suppliertoken0001", http.StatusForbidden},
		{"supplier blocked from root", "/root", "suppliertoken0001", http.StatusForbidden},
		{"admin hits supplier (>=)", "/supplier", "admintoken00000001", http.StatusOK},
		{"admin hits admin", "/admin", "admintoken00000001", http.StatusOK},
		{"admin blocked from root", "/root", "admintoken00000001", http.StatusForbidden},
		{"no token unauthorized", "/supplier", "", http.StatusUnauthorized},
		{"bad token unauthorized", "/supplier", "nonexistenttoken00", http.StatusUnauthorized},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if w := req(r, tc.path, tc.token); w.Code != tc.wantStatus {
				t.Errorf("%s %s: status = %d, want %d (body: %s)",
					tc.path, tc.token, w.Code, tc.wantStatus, w.Body.String())
			}
		})
	}
}

func TestRBAC_DisabledAccountRejected(t *testing.T) {
	r := newRouter(t)
	tok := "disabledtoken0001"
	u := &model.User{Username: "disabled", Role: constant.RoleSupplier, Status: constant.StatusDisabled, AccessToken: &tok}
	if err := u.Create("password123"); err != nil {
		t.Fatalf("seed: %v", err)
	}
	if w := req(r, "/supplier", tok); w.Code != http.StatusForbidden {
		t.Errorf("disabled account: status = %d, want 403", w.Code)
	}
}

package httpapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/ilyast91/CLIProxyNew/internal/store"
)

func TestAdminModelHandlerUpsertUsesActorAndAlias(t *testing.T) {
	gin.SetMode(gin.TestMode)
	s := &fakeAdminModelStore{override: store.ModelOverride{ID: 7, Provider: "openai", ModelAlias: "fast", UpstreamModel: "gpt-5", Enabled: true}}
	r := gin.New()
	r.Use(func(c *gin.Context) { c.Set(ContextUserID, int64(42)) })
	r.PUT("/api/v1/admin/models/:modelAlias", NewAdminModelHandler(s).Upsert)
	req := httptest.NewRequest(http.MethodPut, "/api/v1/admin/models/fast", strings.NewReader(`{"provider":"openai","upstream_model":"gpt-5","enabled":true}`))
	req.Header.Set("Content-Type", "application/json")
	res := httptest.NewRecorder()
	r.ServeHTTP(res, req)
	if res.Code != http.StatusOK || s.actor != 42 || s.params.ModelAlias != "fast" || s.params.UpstreamModel != "gpt-5" {
		t.Fatalf("status=%d actor=%d params=%+v", res.Code, s.actor, s.params)
	}
}
func TestAdminModelHandlerDeleteMapsNotFound(t *testing.T) {
	gin.SetMode(gin.TestMode)
	s := &fakeAdminModelStore{deleteErr: store.ErrNotFound}
	r := gin.New()
	r.Use(func(c *gin.Context) { c.Set(ContextUserID, int64(42)) })
	r.DELETE("/api/v1/admin/models/:modelAlias", NewAdminModelHandler(s).Delete)
	res := httptest.NewRecorder()
	r.ServeHTTP(res, httptest.NewRequest(http.MethodDelete, "/api/v1/admin/models/missing", nil))
	if res.Code != http.StatusNotFound || s.deleted != "missing" {
		t.Fatalf("status=%d alias=%q", res.Code, s.deleted)
	}
}

type fakeAdminModelStore struct {
	rows      []store.ModelOverride
	override  store.ModelOverride
	actor     int64
	params    store.UpsertModelOverrideParams
	deleteErr error
	deleted   string
}

func (s *fakeAdminModelStore) List(context.Context) ([]store.ModelOverride, error) {
	return s.rows, nil
}
func (s *fakeAdminModelStore) UpsertWithAudit(_ context.Context, a int64, p store.UpsertModelOverrideParams, _ *netip.Addr) (store.ModelOverride, error) {
	s.actor = a
	s.params = p
	return s.override, nil
}
func (s *fakeAdminModelStore) DeleteWithAudit(_ context.Context, _ int64, a string, _ *netip.Addr) error {
	s.deleted = a
	return s.deleteErr
}

package route

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"apexy/config"
)

func TestMatchRoute(t *testing.T) {
	tests := []struct {
		name      string
		routePath string
		routeMeth string
		reqPath   string
		reqMethod string
		wantMatch bool
	}{
		{"精确匹配", "/api/v1/users", "", "/api/v1/users", "GET", true},
		{"精确匹配-方法一致", "/api/v1/users", "GET", "/api/v1/users", "GET", true},
		{"精确匹配-方法不一致", "/api/v1/users", "POST", "/api/v1/users", "GET", false},
		{"方法忽略大小写", "/api/v1/users", "get", "/api/v1/users", "GET", true},
		{"前缀匹配-子路径", "/api/v1/users", "", "/api/v1/users/123", "GET", true},
		{"前缀匹配-多级子路径", "/api/fallback", "", "/api/fallback/a/b/c", "GET", true},
		{"前缀边界-同前缀不同段不匹配", "/api/v1/users", "", "/api/v1/users-extra", "GET", false},
		{"路由路径以斜杠结尾-匹配子路径", "/api/v1/", "", "/api/v1/anything", "GET", true},
		{"路由路径以斜杠结尾-精确路径少斜杠不匹配", "/api/v1/", "", "/api/v1", "GET", false},
		{"完全不同的路径", "/api/v1/users", "", "/other", "GET", false},
		{"空路由路径不匹配", "", "", "/api/v1/users", "GET", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			route := &config.Route{Path: tt.routePath, Method: tt.routeMeth}
			req := httptest.NewRequest(tt.reqMethod, tt.reqPath, nil)
			if got := matchRoute(req, route); got != tt.wantMatch {
				t.Errorf("matchRoute(%s %s, route{%s %s}) = %v, want %v",
					tt.reqMethod, tt.reqPath, tt.routeMeth, tt.routePath, got, tt.wantMatch)
			}
		})
	}
}

func TestRouterMatchRouteOrder(t *testing.T) {
	router := NewRouter()
	boolPtr := func(b bool) *bool { return &b }
	_ = boolPtr
	if err := router.LoadRoutesFromConfig([]config.Route{
		{Path: "/api/v1/users", Method: "GET", Backend: "http://b1"},
		{Path: "/api/v1", Method: "", Backend: "http://b2"},
	}); err != nil {
		t.Fatalf("LoadRoutesFromConfig failed: %v", err)
	}

	// 按配置顺序匹配, 第一条命中的路由生效
	req := httptest.NewRequest("GET", "/api/v1/users/1", nil)
	matched := router.MatchRoute(req)
	if matched == nil || matched.Backend != "http://b1" {
		t.Fatalf("expected backend http://b1, got %+v", matched)
	}

	req2 := httptest.NewRequest("GET", "/api/v1/other", nil)
	matched2 := router.MatchRoute(req2)
	if matched2 == nil || matched2.Backend != "http://b2" {
		t.Fatalf("expected backend http://b2, got %+v", matched2)
	}

	req3 := httptest.NewRequest("GET", "/nope", nil)
	if matched3 := router.MatchRoute(req3); matched3 != nil {
		t.Fatalf("expected nil route, got %+v", matched3)
	}
}

func TestRouteContext(t *testing.T) {
	routeEntry := &config.Route{Path: "/api/v1/users", Backend: "http://b1"}

	ctx := WithRoute(context.Background(), routeEntry)
	got := FromContext(ctx)
	if got != routeEntry {
		t.Fatalf("FromContext returned %+v, want %+v", got, routeEntry)
	}

	// 未注入路由时返回 nil
	if got := FromContext(context.Background()); got != nil {
		t.Fatalf("expected nil from empty context, got %+v", got)
	}
}

func TestGetRoutesReturnsCopy(t *testing.T) {
	router := NewRouter()
	if err := router.LoadRoutesFromConfig([]config.Route{{Path: "/a", Backend: "http://a"}}); err != nil {
		t.Fatal(err)
	}
	routes := router.GetRoutes()
	routes[0].Path = "/mutated" // 修改返回值不应影响内部状态
	if router.GetRoutes()[0].Path != "/a" {
		t.Fatal("GetRoutes did not return a copy")
	}
}

var _ = http.MethodGet

package plugin

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// fakePlugin 测试用插件: 在响应 Header 中留下标记
type fakePlugin struct {
	name string
}

func (p *fakePlugin) Name() string { return p.name }

func (p *fakePlugin) Handle(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Plugin", p.name)
		next.ServeHTTP(w, r)
	})
}

// resetRegistry 清空全局注册表, 保证用例间隔离
func resetRegistry() {
	registryMu.Lock()
	defer registryMu.Unlock()
	registry = nil
}

func TestRegisterAndPlugins(t *testing.T) {
	resetRegistry()
	defer resetRegistry()

	if err := Register(&fakePlugin{name: "p1"}); err != nil {
		t.Fatalf("Register p1 failed: %v", err)
	}
	if err := Register(&fakePlugin{name: "p2"}); err != nil {
		t.Fatalf("Register p2 failed: %v", err)
	}

	// 重复注册同名插件应报错
	if err := Register(&fakePlugin{name: "p1"}); err == nil {
		t.Error("expected error for duplicate plugin name")
	}

	plugins := Plugins()
	if len(plugins) != 2 {
		t.Fatalf("len(plugins) = %d, want 2", len(plugins))
	}
	// 按注册顺序返回
	if plugins[0].Name() != "p1" || plugins[1].Name() != "p2" {
		t.Errorf("unexpected order: %s, %s", plugins[0].Name(), plugins[1].Name())
	}

	// Plugins 返回副本, 修改不影响注册表
	plugins[0] = nil
	if Plugins()[0] == nil {
		t.Error("Plugins did not return a copy")
	}
}

func TestMustRegisterPanicsOnDuplicate(t *testing.T) {
	resetRegistry()
	defer resetRegistry()

	MustRegister(&fakePlugin{name: "dup"})
	defer func() {
		if r := recover(); r == nil {
			t.Error("MustRegister should panic on duplicate name")
		}
	}()
	MustRegister(&fakePlugin{name: "dup"})
}

func TestPluginHandleWrapsHandler(t *testing.T) {
	resetRegistry()
	defer resetRegistry()

	MustRegister(&fakePlugin{name: "marker"})

	// 模拟 main.go 的接入方式: 将插件按注册顺序包装到最终 Handler 上
	final := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	var handler http.Handler = final
	plugins := Plugins()
	for i := len(plugins) - 1; i >= 0; i-- {
		handler = plugins[i].Handle(handler)
	}

	req := httptest.NewRequest("GET", "/x", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if got := w.Header().Get("X-Plugin"); got != "marker" {
		t.Errorf("X-Plugin = %q, want marker", got)
	}
	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", w.Code)
	}
}

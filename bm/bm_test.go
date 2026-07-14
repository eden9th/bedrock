package bm_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/eden9th/bedrock/bm"
)

func newEngine(routes func(e *bm.Engine)) *bm.Engine {
	e := bm.New()
	routes(e)
	return e
}

func TestRouting_200(t *testing.T) {
	e := newEngine(func(e *bm.Engine) {
		e.GET("/hello", func(c *bm.Context) { c.String(200, "ok") })
	})

	req := httptest.NewRequest(http.MethodGet, "/hello", nil)
	w := httptest.NewRecorder()
	e.ServeHTTP(w, req)

	if w.Code != 200 {
		t.Fatalf("expected 200, got %d", w.Code)
	}
}

func TestRouting_404(t *testing.T) {
	e := newEngine(func(e *bm.Engine) {
		e.GET("/hello", func(c *bm.Context) { c.String(200, "ok") })
	})

	req := httptest.NewRequest(http.MethodGet, "/notfound", nil)
	w := httptest.NewRecorder()
	e.ServeHTTP(w, req)

	if w.Code != 404 {
		t.Fatalf("expected 404, got %d", w.Code)
	}
}

func TestRouting_405(t *testing.T) {
	e := newEngine(func(e *bm.Engine) {
		e.GET("/hello", func(c *bm.Context) { c.String(200, "ok") })
	})

	req := httptest.NewRequest(http.MethodPost, "/hello", nil)
	w := httptest.NewRecorder()
	e.ServeHTTP(w, req)

	if w.Code != 405 {
		t.Fatalf("expected 405, got %d", w.Code)
	}
}

func TestPathParam(t *testing.T) {
	e := newEngine(func(e *bm.Engine) {
		e.GET("/users/:id", func(c *bm.Context) {
			c.String(200, c.Param("id"))
		})
	})

	req := httptest.NewRequest(http.MethodGet, "/users/42", nil)
	w := httptest.NewRecorder()
	e.ServeHTTP(w, req)

	if w.Code != 200 {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if w.Body.String() != "42" {
		t.Fatalf("expected param id=42, got %q", w.Body.String())
	}
}

func TestWildcardParam(t *testing.T) {
	e := newEngine(func(e *bm.Engine) {
		e.GET("/files/*filepath", func(c *bm.Context) {
			c.String(200, c.Param("filepath"))
		})
	})

	req := httptest.NewRequest(http.MethodGet, "/files/a/b/c.txt", nil)
	w := httptest.NewRecorder()
	e.ServeHTTP(w, req)

	if w.Code != 200 {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "a/b/c.txt") {
		t.Fatalf("expected wildcard filepath, got %q", w.Body.String())
	}
}

func TestMiddleware_Order(t *testing.T) {
	var order []string
	e := bm.New()
	e.Use(func(c *bm.Context) {
		order = append(order, "mw1")
		c.Next()
	})
	e.Use(func(c *bm.Context) {
		order = append(order, "mw2")
		c.Next()
	})
	e.GET("/ping", func(c *bm.Context) {
		order = append(order, "handler")
		c.String(200, "pong")
	})

	req := httptest.NewRequest(http.MethodGet, "/ping", nil)
	w := httptest.NewRecorder()
	e.ServeHTTP(w, req)

	expected := []string{"mw1", "mw2", "handler"}
	if len(order) != len(expected) {
		t.Fatalf("expected order %v, got %v", expected, order)
	}
	for i, v := range expected {
		if order[i] != v {
			t.Fatalf("expected order[%d]=%q, got %q", i, v, order[i])
		}
	}
}

func TestRouterGroup_PrefixAndMiddleware(t *testing.T) {
	var mwCalled bool
	e := bm.New()
	g := e.Group("/api", func(c *bm.Context) {
		mwCalled = true
		c.Next()
	})
	g.GET("/items", func(c *bm.Context) {
		c.String(200, "items")
	})

	req := httptest.NewRequest(http.MethodGet, "/api/items", nil)
	w := httptest.NewRecorder()
	e.ServeHTTP(w, req)

	if w.Code != 200 {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if !mwCalled {
		t.Fatal("group middleware was not called")
	}
}

func TestContext_SetGet(t *testing.T) {
	e := bm.New()
	var gotVal any
	var gotOk bool

	e.GET("/kv", func(c *bm.Context) {
		c.Set("mykey", "myvalue")
		c.Set("mykey", "overwritten") // 覆盖
		gotVal, gotOk = c.Get("mykey")
		c.String(200, "ok")
	})

	req := httptest.NewRequest(http.MethodGet, "/kv", nil)
	w := httptest.NewRecorder()
	e.ServeHTTP(w, req)

	if !gotOk {
		t.Fatal("Get returned ok=false")
	}
	if gotVal != "overwritten" {
		t.Fatalf("expected %q, got %v", "overwritten", gotVal)
	}
}

func TestContext_Get_MissingKey(t *testing.T) {
	e := bm.New()
	var gotVal any
	var gotOk bool

	e.GET("/kv", func(c *bm.Context) {
		gotVal, gotOk = c.Get("nonexistent")
		c.String(200, "ok")
	})

	req := httptest.NewRequest(http.MethodGet, "/kv", nil)
	w := httptest.NewRecorder()
	e.ServeHTTP(w, req)

	if gotOk {
		t.Fatal("expected ok=false for missing key")
	}
	if gotVal != nil {
		t.Fatalf("expected nil for missing key, got %v", gotVal)
	}
}

func TestAbort_StopsChain(t *testing.T) {
	var secondCalled bool
	e := bm.New()
	e.Use(func(c *bm.Context) {
		c.AbortWithStatus(403)
	})
	e.GET("/secret", func(c *bm.Context) {
		secondCalled = true
		c.String(200, "ok")
	})

	req := httptest.NewRequest(http.MethodGet, "/secret", nil)
	w := httptest.NewRecorder()
	e.ServeHTTP(w, req)

	if w.Code != 403 {
		t.Fatalf("expected 403, got %d", w.Code)
	}
	if secondCalled {
		t.Fatal("handler should not be called after Abort")
	}
}

func TestJSON_Response(t *testing.T) {
	e := bm.New()
	e.GET("/json", func(c *bm.Context) {
		c.JSON(map[string]string{"key": "val"}, nil)
	})

	req := httptest.NewRequest(http.MethodGet, "/json", nil)
	w := httptest.NewRecorder()
	e.ServeHTTP(w, req)

	if w.Code != 200 {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var resp struct {
		Code    int               `json:"code"`
		Message string            `json:"message"`
		Data    map[string]string `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse JSON: %v", err)
	}
	if resp.Code != 0 {
		t.Fatalf("expected code=0, got %d", resp.Code)
	}
	if resp.Data["key"] != "val" {
		t.Fatalf("expected data.key=val, got %q", resp.Data["key"])
	}
}

func TestConcurrentRouteRegistration_NoRace(t *testing.T) {
	e := bm.New()
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			e.GET("/ping", func(c *bm.Context) { c.String(200, "pong") })
		}(i)
	}
	wg.Wait()
	// 发一个请求确认引擎正常
	req := httptest.NewRequest(http.MethodGet, "/ping", nil)
	w := httptest.NewRecorder()
	e.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatalf("expected 200 after concurrent registration, got %d", w.Code)
	}
}

func TestCORS_PreflightForRegisteredPath(t *testing.T) {
	e := bm.New()
	e.Use(bm.CORS(bm.CORSConfig{
		AllowOrigins: []string{"https://example.com"},
		AllowMethods: []string{"GET", "POST", "OPTIONS"},
		AllowHeaders: []string{"Content-Type"},
	}))
	e.GET("/hello", func(c *bm.Context) { c.String(200, "ok") })

	req := httptest.NewRequest(http.MethodOptions, "/hello", nil)
	req.Header.Set("Origin", "https://example.com")
	req.Header.Set("Access-Control-Request-Method", "GET")
	w := httptest.NewRecorder()
	e.ServeHTTP(w, req)

	if w.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d", w.Code)
	}
	if got := w.Header().Get("Access-Control-Allow-Origin"); got != "https://example.com" {
		t.Fatalf("expected reflected origin, got %q", got)
	}
}

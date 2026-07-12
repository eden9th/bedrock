package httputil_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/eden9th/bedrock/bm"
	"github.com/eden9th/bedrock/httputil"
)

func serve(handler bm.HandlerFunc, method, path string, body string) *httptest.ResponseRecorder {
	e := bm.New()
	switch method {
	case http.MethodGet:
		e.GET(path, handler)
	case http.MethodPost:
		e.POST(path, handler)
	}
	var req *http.Request
	if body != "" {
		req = httptest.NewRequest(method, path, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
	} else {
		req = httptest.NewRequest(method, path, nil)
	}
	w := httptest.NewRecorder()
	e.ServeHTTP(w, req)
	return w
}

func TestJSON_OutputsBareJSON(t *testing.T) {
	w := serve(func(c *bm.Context) {
		httputil.JSON(c, map[string]int{"count": 3})
	}, http.MethodGet, "/test", "")

	if w.Code != 200 {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	ct := w.Header().Get("Content-Type")
	if !strings.Contains(ct, "application/json") {
		t.Fatalf("expected JSON content-type, got %q", ct)
	}
	var out map[string]int
	if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}
	if out["count"] != 3 {
		t.Fatalf("expected count=3, got %d", out["count"])
	}
	// 确保是裸 JSON，不含信封
	if _, hasCode := out["code"]; hasCode {
		t.Fatal("httputil.JSON should output bare JSON without envelope")
	}
}

func TestJSONError_StatusAndAbort(t *testing.T) {
	e := bm.New()
	e.GET("/err", func(c *bm.Context) {
		httputil.JSONError(c, http.StatusBadRequest, "invalid input")
		// JSONError 调用 c.Abort()，中止后续中间件链；
		// 但同一 handler 内后续代码仍会执行，需要 return 阻止
		return
	})

	req := httptest.NewRequest(http.MethodGet, "/err", nil)
	w := httptest.NewRecorder()
	e.ServeHTTP(w, req)

	if w.Code != 400 {
		t.Fatalf("expected 400, got %d", w.Code)
	}
	var out map[string]string
	if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
		t.Fatalf("failed to parse error response: %v\nbody: %s", err, w.Body.String())
	}
	if out["detail"] != "invalid input" {
		t.Fatalf("expected detail=%q, got %q", "invalid input", out["detail"])
	}
}

func TestBind_Success(t *testing.T) {
	type Req struct {
		Name string `json:"name"`
		Age  int    `json:"age"`
	}
	var parsed Req

	e := bm.New()
	e.POST("/bind", func(c *bm.Context) {
		if !httputil.Bind(c, &parsed) {
			return
		}
		c.String(200, "ok")
	})

	body := `{"name":"alice","age":30}`
	req := httptest.NewRequest(http.MethodPost, "/bind", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	e.ServeHTTP(w, req)

	if w.Code != 200 {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if parsed.Name != "alice" || parsed.Age != 30 {
		t.Fatalf("parsed unexpected values: %+v", parsed)
	}
}

func TestBind_InvalidJSON_Returns400(t *testing.T) {
	type Req struct {
		Name string `json:"name"`
	}

	e := bm.New()
	e.POST("/bind", func(c *bm.Context) {
		var r Req
		if !httputil.Bind(c, &r) {
			return
		}
		c.String(200, "should not reach")
	})

	req := httptest.NewRequest(http.MethodPost, "/bind", strings.NewReader("not-json"))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	e.ServeHTTP(w, req)

	if w.Code != 400 {
		t.Fatalf("expected 400 for invalid JSON, got %d", w.Code)
	}
}

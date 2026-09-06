package mcp_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"

	"github.com/roarc0/squirrel/backend/internal/mcp"
)

type dummyHandler struct{}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (fn roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return fn(req)
}

func (d *dummyHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path == "/v1.SummaryService/GetSummary" {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"summary":{"baseCurrency":"EUR"}}`))
		return
	}
	http.NotFound(w, r)
}

func TestMCPHandlerGET(t *testing.T) {
	handler := mcp.NewHandler(&dummyHandler{})
	req := httptest.NewRequest(http.MethodGet, "/mcp", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}

	var resp map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}

	if resp["name"] != "squirrel-mcp" {
		t.Errorf("expected server name squirrel-mcp, got %v", resp["name"])
	}
}

func TestMCPHandlerInitialize(t *testing.T) {
	handler := mcp.NewHandler(&dummyHandler{})
	body := []byte(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}`)
	req := httptest.NewRequest(http.MethodPost, "/mcp", bytes.NewReader(body))
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}

	var resp map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}

	result, ok := resp["result"].(map[string]interface{})
	if !ok || result["protocolVersion"] != "2024-11-05" {
		t.Errorf("invalid initialize result: %v", resp)
	}
}

func TestMCPHandlerToolsList(t *testing.T) {
	handler := mcp.NewHandler(&dummyHandler{})
	body := []byte(`{"jsonrpc":"2.0","id":2,"method":"tools/list","params":{}}`)
	req := httptest.NewRequest(http.MethodPost, "/mcp", bytes.NewReader(body))
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}

	var resp map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}

	result := resp["result"].(map[string]interface{})
	tools := result["tools"].([]interface{})
	if len(tools) == 0 {
		t.Errorf("expected tools list to contain tools, got 0")
	}
}

func TestMCPToolsAreReadOnly(t *testing.T) {
	handler := mcp.NewHandler(&dummyHandler{})
	for _, tool := range handler.OpenAITools() {
		fn := tool["function"].(map[string]interface{})
		name := fn["name"].(string)
		for _, prefix := range []string{"create_", "update_", "delete_", "save_", "restore_", "import_", "sync_", "enrich_", "star_", "refresh_"} {
			if strings.HasPrefix(name, prefix) {
				t.Fatalf("AI tool %q is not read-only", name)
			}
		}
	}
}

func TestMCPHandlerDoesNotOverrideCORS(t *testing.T) {
	handler := mcp.NewHandler(&dummyHandler{})
	body := []byte(`{"jsonrpc":"2.0","id":2,"method":"tools/list","params":{}}`)
	req := httptest.NewRequest(http.MethodPost, "/mcp", bytes.NewReader(body))
	req.Header.Set("Origin", "https://evil.example")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Fatalf("MCP handler bypassed server CORS policy with %q", got)
	}
}

func TestMCPHandlerToolsCall(t *testing.T) {
	handler := mcp.NewHandler(&dummyHandler{})
	body := []byte(`{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"get_summary","arguments":{}}}`)
	req := httptest.NewRequest(http.MethodPost, "/mcp", bytes.NewReader(body))
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}

	var resp map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}

	result := resp["result"].(map[string]interface{})
	content := result["content"].([]interface{})
	if len(content) == 0 {
		t.Fatalf("expected content in result")
	}
}

func TestMCPWebSearchTool(t *testing.T) {
	searchServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/html/" {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write([]byte(`<a class="result__a" href="https://example.com/btp">Italian BTP yield</a><a class="result__snippet">Deterministic search result</a>`))
	}))
	t.Cleanup(searchServer.Close)

	target, err := url.Parse(searchServer.URL)
	if err != nil {
		t.Fatal(err)
	}
	transport := searchServer.Client().Transport
	previousTransport := http.DefaultTransport
	http.DefaultTransport = roundTripFunc(func(req *http.Request) (*http.Response, error) {
		clone := req.Clone(req.Context())
		clone.URL.Scheme = target.Scheme
		clone.URL.Host = target.Host
		return transport.RoundTrip(clone)
	})
	t.Cleanup(func() { http.DefaultTransport = previousTransport })

	handler := mcp.NewHandler(&dummyHandler{})
	body := []byte(`{"jsonrpc":"2.0","id":4,"method":"tools/call","params":{"name":"web_search","arguments":{"query":"Italian BTP yield"}}}`)
	req := httptest.NewRequest(http.MethodPost, "/mcp", bytes.NewReader(body))
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}

	var resp map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}

	result, ok := resp["result"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected result object")
	}

	content, ok := result["content"].([]interface{})
	if !ok || len(content) == 0 {
		t.Fatalf("expected non-empty content in result: %v", result)
	}
	if !strings.Contains(rec.Body.String(), "https://example.com/btp") {
		t.Fatalf("expected deterministic search result: %s", rec.Body.String())
	}
}

func TestLiveMCPWebSearchTool(t *testing.T) {
	if os.Getenv("SQUIRREL_INTEGRATION") != "1" {
		t.Skip("set SQUIRREL_INTEGRATION=1 to run live web search")
	}
	result, err := mcp.PerformWebSearch(context.Background(), "Italian BTP yield")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(result, `"results_count"`) {
		t.Fatalf("unexpected live search result: %s", result)
	}
}

package passive

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestFetch(t *testing.T) {
	// Mock Wayback response (CDX JSON format)
	waybackResponse := `[
		["original"],
		["http://test.com/page1"],
		["https://test.com/page2?id=123"]
	]`

	// Mock OTX response (JSON)
	otxResponse := `{
		"url_list": [
			{"url": "http://test.com/page3"},
			{"url": "https://test.com/page2?id=123"}
		],
		"has_next": false
	}`

	// Create test server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.RawQuery, "url") || strings.Contains(r.URL.Path, "cdx") {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(waybackResponse))
			return
		}
		if strings.Contains(r.URL.Path, "url_list") {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(otxResponse))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	// Override API endpoints to use mock server
	oldWaybackBase := waybackURLBase
	oldOtxBase := otxURLBase
	defer func() {
		waybackURLBase = oldWaybackBase
		otxURLBase = oldOtxBase
	}()

	waybackURLBase = server.URL + "/cdx"
	otxURLBase = server.URL + "/indicators/domain"

	ctx := context.Background()
	urls, err := Fetch(ctx, "test.com")
	if err != nil {
		t.Fatalf("Fetch failed: %s", err)
	}

	// Expecting 3 unique URLs: page1, page2, page3
	expectedCount := 3
	if len(urls) != expectedCount {
		t.Errorf("Expected %d unique URLs, got %d: %v", expectedCount, len(urls), urls)
	}

	foundPage1 := false
	foundPage2 := false
	foundPage3 := false
	for _, u := range urls {
		if u == "http://test.com/page1" {
			foundPage1 = true
		}
		if u == "https://test.com/page2?id=123" {
			foundPage2 = true
		}
		if u == "http://test.com/page3" {
			foundPage3 = true
		}
	}

	if !foundPage1 || !foundPage2 || !foundPage3 {
		t.Errorf("Expected URLs not found. Got: %v", urls)
	}
}

func TestCleanDomain(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"test.com", "test.com"},
		{"http://test.com", "test.com"},
		{"https://sub.test.com/path", "sub.test.com"},
		{"  test.com  ", "test.com"},
	}

	for _, tc := range tests {
		got := cleanDomain(tc.input)
		if got != tc.expected {
			t.Errorf("cleanDomain(%q) = %q; expected %q", tc.input, got, tc.expected)
		}
	}
}

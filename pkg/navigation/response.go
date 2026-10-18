package navigation

import (
	"net/http"
	"net/url"
	"strings"

	"github.com/PuerkitoBio/goquery"
	jsoniter "github.com/json-iterator/go"
)

type Headers map[string]string

type Form struct {
	Method     string   `json:"method,omitempty"`
	Action     string   `json:"action,omitempty"`
	Enctype    string   `json:"enctype,omitempty"`
	Parameters []string `json:"parameters,omitempty"`
}

func (h *Headers) MarshalJSON() ([]byte, error) {
	hCopy := make(Headers)
	for k, v := range *h {
		k := strings.ToLower(k)
		hCopy[k] = v
	}
	return jsoniter.Marshal(hCopy)
}

// Response is a response generated from crawler navigation
type Response struct {
	Resp               *http.Response    `json:"-"`
	Depth              int               `json:"-"`
	Reader             *goquery.Document `json:"-"`
	StatusCode         int               `json:"status_code,omitempty"`
	Headers            Headers           `json:"headers,omitempty"`
	Body               string            `json:"body,omitempty"`
	ContentLength      int64             `json:"content_length,omitempty"`
	RootHostname       string            `json:"-"`
	Technologies       []string          `json:"technologies,omitempty"`
	Raw                string            `json:"raw,omitempty"`
	Forms              []Form            `json:"forms,omitempty"`
	XhrRequests        []Request         `json:"xhr_requests,omitempty"`
	StoredResponsePath string            `json:"stored_response_path,omitempty"`
	KnowledgeBase      map[string]any    `json:"knowledgebase,omitempty"`
}

func (n Response) AbsoluteURL(path string) string {
	if strings.HasPrefix(path, "#") {
		return ""
	}

	var baseURL *url.URL
	if n.Resp != nil && n.Resp.Request != nil && n.Resp.Request.URL != nil {
		baseURL = n.Resp.Request.URL
	} else if n.Reader != nil && n.Reader.Url != nil {
		baseURL = n.Reader.Url
	}

	if baseURL == nil {
		parsed, err := url.Parse(path)
		if err == nil && parsed.IsAbs() {
			parsed.Fragment = ""
			return parsed.String()
		}
		return ""
	}

	absURL, err := baseURL.Parse(path)
	if err != nil {
		return ""
	}
	absURL.Fragment = ""
	if absURL.Scheme == "" && baseURL.Scheme != "" {
		absURL.Scheme = baseURL.Scheme
	}
	final := absURL.String()
	return final
}

func (n Response) IsRedirect() bool {
	return n.StatusCode >= 300 && n.StatusCode <= 399
}

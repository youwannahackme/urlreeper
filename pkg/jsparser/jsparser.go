package jsparser

import (
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
)

// LinkFinderRegex is the Go port of Gerben Javado's LinkFinder regex.
// It matches absolute URLs, relative paths, REST endpoints, and common files in JS strings.
var LinkFinderRegex = regexp.MustCompile(`(?:"|')` +
	`(` +
	`((?:[a-zA-Z]{1,10}://|//)[^"'/]{1,}\.[a-zA-Z]{2,}[^"']{0,})` + // Absolute/protocol-relative
	`|` +
	`((?:/|\.\./|\./)[^"'><,;| *()(\%^\/\\\[\]][^"'><,;|()]{1,})` + // Relative path
	`|` +
	`([a-zA-Z0-9_\-/]{1,}/[a-zA-Z0-9_\-/.]{1,}\.(?:[a-zA-Z]{1,4}|action)(?:[\?|#][^"|']{0,})?)` + // Relative path with extension
	`|` +
	`([a-zA-Z0-9_\-/]{1,}/[a-zA-Z0-9_\-/]{3,}(?:[\?|#][^"|']{0,})?)` + // REST API (no extension)
	`|` +
	`([a-zA-Z0-9_\-]{1,}\.(?:php|asp|aspx|jsp|json|action|html|js|txt|xml)(?:[\?|#][^"|']{0,})?)` + // Filename with extension
	`)` +
	`(?:"|')`)

// ExtractEndpoints parses JS content and returns all unique discovered endpoints
func ExtractEndpoints(content string) []string {
	matches := LinkFinderRegex.FindAllStringSubmatch(content, -1)
	unique := make(map[string]bool)
	var results []string

	for _, match := range matches {
		if len(match) > 1 {
			endpoint := match[1]
			endpoint = strings.TrimSpace(endpoint)
			if endpoint != "" && !unique[endpoint] {
				unique[endpoint] = true
				results = append(results, endpoint)
			}
		}
	}
	return results
}

// ResolveURL resolves a relative URL against a base URL
func ResolveURL(baseURLStr, relativeURLStr string) string {
	if strings.HasPrefix(relativeURLStr, "mailto:") || strings.HasPrefix(relativeURLStr, "javascript:") {
		return ""
	}
	u, err := url.Parse(relativeURLStr)
	if err != nil {
		return ""
	}
	if u.IsAbs() {
		return relativeURLStr
	}

	baseURL, err := url.Parse(baseURLStr)
	if err != nil {
		return relativeURLStr
	}

	// Handle protocol-relative URLs (e.g. //example.com)
	if strings.HasPrefix(relativeURLStr, "//") {
		return baseURL.Scheme + ":" + relativeURLStr
	}

	resolved := baseURL.ResolveReference(u)
	return resolved.String()
}

// FetchJSContent downloads the content of a JS URL using the provided HTTP client
func FetchJSContent(client *http.Client, jsURL string, headers map[string]string) (string, error) {
	req, err := http.NewRequest("GET", jsURL, nil)
	if err != nil {
		return "", err
	}

	// Set default User-Agent and any custom headers
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/115.0.0.0 Safari/537.36")
	for k, v := range headers {
		req.Header.Set(k, v)
	}

	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("bad status code: %d", resp.StatusCode)
	}

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	return string(bodyBytes), nil
}

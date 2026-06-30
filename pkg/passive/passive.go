package passive

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path/filepath"
	"sort"
	"strings"
	"sync"
)

var staticExtensions = map[string]bool{
	".jpg":   true,
	".jpeg":  true,
	".png":   true,
	".gif":   true,
	".pdf":   true,
	".svg":   true,
	".json":  true,
	".css":   true,
	".js":    true,
	".webp":  true,
	".woff":  true,
	".woff2": true,
	".eot":   true,
	".ttf":   true,
	".otf":   true,
	".mp4":   true,
	".txt":   true,
	".ico":   true,
	".zip":   true,
	".tar":   true,
	".gz":    true,
	".rar":   true,
}

// OTXResponse represents the JSON structure from AlienVault OTX
type OTXResponse struct {
	URLList []struct {
		URL string `json:"url"`
	} `json:"url_list"`
	HasNext bool `json:"has_next"`
}

// URLScanResponse represents the JSON structure from URLScan
type URLScanResponse struct {
	Results []struct {
		Page struct {
			URL string `json:"url"`
		} `json:"page"`
	} `json:"results"`
}

// FetchPassiveURLs gathers URLs from Wayback, OTX, and URLScan concurrently
func FetchPassiveURLs(ctx context.Context, client *http.Client, domain string, subs bool) []string {
	var wg sync.WaitGroup
	urlChan := make(chan string, 1000)

	// 1. Wayback Machine
	wg.Add(1)
	go func() {
		defer wg.Done()
		var urls []string

		// Always query the root domain
		rootURL := fmt.Sprintf("https://web.archive.org/cdx/search/cdx?url=%s/*&output=txt&collapse=urlkey&fl=original", domain)
		req1, err := http.NewRequestWithContext(ctx, "GET", rootURL, nil)
		if err == nil {
			resp1, err := client.Do(req1)
			if err == nil {
				body, err := io.ReadAll(resp1.Body)
				resp1.Body.Close()
				if err == nil {
					urls = append(urls, strings.Split(string(body), "\n")...)
				}
			}
		}

		// Query subdomains if subs is true
		if subs {
			subURL := fmt.Sprintf("https://web.archive.org/cdx/search/cdx?url=*.%s/*&output=txt&collapse=urlkey&fl=original", domain)
			req2, err := http.NewRequestWithContext(ctx, "GET", subURL, nil)
			if err == nil {
				resp2, err := client.Do(req2)
				if err == nil {
					body, err := io.ReadAll(resp2.Body)
					resp2.Body.Close()
					if err == nil {
						urls = append(urls, strings.Split(string(body), "\n")...)
					}
				}
			}
		}

		for _, line := range urls {
			line = strings.TrimSpace(line)
			if line != "" {
				select {
				case urlChan <- line:
				case <-ctx.Done():
					return
				}
			}
		}
	}()

	// 2. AlienVault OTX
	wg.Add(1)
	go func() {
		defer wg.Done()
		page := 1
		for {
			otxURL := fmt.Sprintf("https://otx.alienvault.com/api/v1/indicators/domain/%s/url_list?limit=500&page=%d", domain, page)
			req, err := http.NewRequestWithContext(ctx, "GET", otxURL, nil)
			if err != nil {
				return
			}
			resp, err := client.Do(req)
			if err != nil {
				return
			}

			var otxResp OTXResponse
			err = json.NewDecoder(resp.Body).Decode(&otxResp)
			resp.Body.Close()
			if err != nil {
				return
			}

			for _, item := range otxResp.URLList {
				select {
				case urlChan <- item.URL:
				case <-ctx.Done():
					return
				}
			}

			if !otxResp.HasNext || page >= 10 { // Limit to 10 pages to avoid hanging
				break
			}
			page++
		}
	}()

	// 3. URLScan
	wg.Add(1)
	go func() {
		defer wg.Done()
		urlscanURL := fmt.Sprintf("https://urlscan.io/api/v1/search/?q=domain:%s&size=100", domain)
		req, err := http.NewRequestWithContext(ctx, "GET", urlscanURL, nil)
		if err != nil {
			return
		}
		resp, err := client.Do(req)
		if err != nil {
			return
		}
		defer resp.Body.Close()

		var urlscanResp URLScanResponse
		if err := json.NewDecoder(resp.Body).Decode(&urlscanResp); err != nil {
			return
		}

		for _, result := range urlscanResp.Results {
			if result.Page.URL != "" {
				select {
				case urlChan <- result.Page.URL:
				case <-ctx.Done():
					return
				}
			}
		}
	}()

	// Close channel when all gatherers are done
	go func() {
		wg.Wait()
		close(urlChan)
	}()

	var results []string
	seen := make(map[string]bool)
	for u := range urlChan {
		parsed, err := url.Parse(u)
		if err != nil {
			continue
		}
		host := strings.ToLower(parsed.Hostname())
		if host == "" {
			continue
		}

		if subs {
			if host != domain && !strings.HasSuffix(host, "."+domain) {
				continue
			}
		} else {
			if host != domain && host != "www."+domain {
				continue
			}
		}

		if !seen[u] {
			seen[u] = true
			results = append(results, u)
		}
	}

	return results
}

// CleanAndProcessURLs processes a list of URLs according to ParamSpider/Gau cleaning rules
func CleanAndProcessURLs(urls []string, placeholder string, paramsOnly bool) []string {
	var processed []string
	seenSignatures := make(map[string]bool)

	for _, rawURL := range urls {
		// 1. Basic parse and validation
		u, err := url.Parse(rawURL)
		if err != nil || !u.IsAbs() {
			continue
		}

		// 2. Remove default ports
		if (u.Scheme == "http" && u.Port() == "80") || (u.Scheme == "https" && u.Port() == "443") {
			u.Host = u.Hostname()
		}

		// 3. Filter out static extensions
		ext := strings.ToLower(filepath.Ext(u.Path))
		if staticExtensions[ext] {
			continue
		}

		queryParams := u.Query()
		hasParams := len(queryParams) > 0

		if paramsOnly && !hasParams {
			continue
		}

		if hasParams {
			// Replace parameter values with the placeholder (e.g. FUZZ)
			newQuery := url.Values{}
			var keys []string
			for key := range queryParams {
				newQuery.Set(key, placeholder)
				keys = append(keys, key)
			}
			u.RawQuery = newQuery.Encode()

			// Create a unique signature based on host, path, and sorted parameter keys
			sort.Strings(keys)
			signature := fmt.Sprintf("%s|%s|%s", u.Host, u.Path, strings.Join(keys, ","))
			if seenSignatures[signature] {
				continue
			}
			seenSignatures[signature] = true
		} else {
			// For non-parameter URLs, deduplicate by the entire URL
			signature := u.String()
			if seenSignatures[signature] {
				continue
			}
			seenSignatures[signature] = true
		}

		processed = append(processed, u.String())
	}

	return processed
}

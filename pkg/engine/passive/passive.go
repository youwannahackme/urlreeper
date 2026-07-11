package passive

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/projectdiscovery/gologger"
)

var (
	waybackURLBase = "https://web.archive.org/cdx/search/cdx"
	otxURLBase     = "https://otx.alienvault.com/api/v1/indicators/domain"
)

// Fetch collects passive URLs for a given domain/host from public archives.
func Fetch(ctx context.Context, target string) ([]string, error) {
	// Extract clean domain name
	domain := cleanDomain(target)
	if domain == "" {
		return nil, fmt.Errorf("invalid domain target: %s", target)
	}

	gologger.Info().Msgf("Fetching passive URLs for domain => %s", domain)

	var wg sync.WaitGroup
	urlChan := make(chan string, 1000)
	errChan := make(chan error, 2)

	// Fetch from Wayback Machine
	wg.Add(1)
	go func() {
		defer wg.Done()
		if err := fetchWayback(ctx, domain, urlChan); err != nil {
			gologger.Debug().Msgf("Wayback fetch failed for %s: %s", domain, err)
			errChan <- err
		}
	}()

	// Fetch from AlienVault OTX
	wg.Add(1)
	go func() {
		defer wg.Done()
		if err := fetchOTX(ctx, domain, urlChan); err != nil {
			gologger.Debug().Msgf("OTX fetch failed for %s: %s", domain, err)
			errChan <- err
		}
	}()

	// Wait for all fetchers to complete and close urlChan
	go func() {
		wg.Wait()
		close(urlChan)
	}()

	// Collect unique URLs
	uniqueURLs := make(map[string]struct{})
	for u := range urlChan {
		u = strings.TrimSpace(u)
		if u != "" {
			uniqueURLs[u] = struct{}{}
		}
	}

	urls := make([]string, 0, len(uniqueURLs))
	for u := range uniqueURLs {
		urls = append(urls, u)
	}

	// Select first error if any, or nil
	select {
	case err := <-errChan:
		// We still return collected URLs even if one source failed
		return urls, err
	default:
		return urls, nil
	}
}

// cleanDomain parses domain from raw input
func cleanDomain(input string) string {
	input = strings.TrimSpace(input)
	if !strings.HasPrefix(input, "http://") && !strings.HasPrefix(input, "https://") {
		input = "http://" + input
	}
	parsed, err := url.Parse(input)
	if err != nil {
		return ""
	}
	return parsed.Hostname()
}

// fetchWayback queries Wayback Machine CDX API
func fetchWayback(ctx context.Context, domain string, out chan<- string) error {
	apiURL := fmt.Sprintf("%s?url=*.%s/*&output=json&fl=original&collapse=urlkey", waybackURLBase, domain)
	req, err := http.NewRequestWithContext(ctx, "GET", apiURL, nil)
	if err != nil {
		return err
	}

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("wayback returned status code %d", resp.StatusCode)
	}

	// CDX output=json is a JSON array of arrays: [["original"], ["url1"], ["url2"], ...]
	var records [][]string
	if err := json.NewDecoder(resp.Body).Decode(&records); err != nil {
		return err
	}

	// Skip header record (index 0)
	for i := 1; i < len(records); i++ {
		if len(records[i]) > 0 {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case out <- records[i][0]:
			}
		}
	}
	return nil
}

type otxResponse struct {
	URLList []struct {
		URL string `json:"url"`
	} `json:"url_list"`
	HasNext bool `json:"has_next"`
}

// fetchOTX queries AlienVault OTX URL list API
func fetchOTX(ctx context.Context, domain string, out chan<- string) error {
	// Query page 1 (up to 100 entries)
	apiURL := fmt.Sprintf("%s/%s/url_list?limit=100&page=1", otxURLBase, domain)
	req, err := http.NewRequestWithContext(ctx, "GET", apiURL, nil)
	if err != nil {
		return err
	}

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("otx returned status code %d", resp.StatusCode)
	}

	var data otxResponse
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return err
	}

	for _, item := range data.URLList {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case out <- item.URL:
		}
	}
	return nil
}

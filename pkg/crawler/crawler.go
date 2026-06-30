package crawler

import (
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strings"
	"sync"
	"time"
	"github.com/youwannahackme/urlreeper/pkg/jsparser"
	"github.com/youwannahackme/urlreeper/pkg/output"
)

var (
	hrefRegex   = regexp.MustCompile(`(?i)href\s*=\s*["']([^"']+)["']`)
	srcRegex    = regexp.MustCompile(`(?i)src\s*=\s*["']([^"']+)["']`)
	userAgents  = []string{
		"Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36",
		"Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36",
		"Mozilla/5.0 (Windows NT 10.0; Win64; x64; rv:109.0) Gecko/20100101 Firefox/121.0",
		"Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.2 Safari/605.1.15",
		"Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Edge/120.0.0.0",
	}
)

type crawlTask struct {
	url   string
	depth int
}

// Crawler manages the active crawling state
type Crawler struct {
	mu             sync.Mutex
	visited        map[string]bool
	client         *http.Client
	maxDepth       int
	inside         bool
	headers        map[string]string
	jsCrawl        bool
	threads        int
	initialPath    string
	allowedRoots   map[string]bool
	results        chan<- *output.Result
	wg             sync.WaitGroup
	verbose        bool
	delay          time.Duration
	rateLimit      int
	rotateUA       bool
	throttle       <-chan time.Time
}

// NewCrawler creates a new active crawler instance
func NewCrawler(client *http.Client, maxDepth int, inside, jsCrawl bool, headers map[string]string, threads int, allowedRoots map[string]bool, results chan<- *output.Result, verbose bool, delay time.Duration, rateLimit int, rotateUA bool) *Crawler {
	var throttle <-chan time.Time
	if rateLimit > 0 {
		throttle = time.Tick(time.Second / time.Duration(rateLimit))
	}
	return &Crawler{
		visited:      make(map[string]bool),
		client:       client,
		maxDepth:     maxDepth,
		inside:       inside,
		jsCrawl:      jsCrawl,
		headers:      headers,
		threads:      threads,
		allowedRoots: allowedRoots,
		results:      results,
		verbose:      verbose,
		delay:        delay,
		rateLimit:    rateLimit,
		rotateUA:     rotateUA,
		throttle:     throttle,
	}
}

// Crawl starts the crawling process for the seed URLs
func (c *Crawler) Crawl(seeds []string) {
	if len(seeds) == 0 {
		return
	}

	// Parse the first seed URL to set initial path scoping
	u, err := url.Parse(seeds[0])
	if err == nil {
		c.initialPath = u.Path
		if !strings.HasSuffix(c.initialPath, "/") {
			// Ensure path scoping works for parent directory
			idx := strings.LastIndex(c.initialPath, "/")
			if idx >= 0 {
				c.initialPath = c.initialPath[:idx+1]
			}
		}
	}

	queue := make(chan crawlTask, 100000)

	// Start workers
	for i := 0; i < c.threads; i++ {
		go c.worker(queue)
	}

	// Queue seed URLs
	for _, seed := range seeds {
		if c.markVisited(seed) {
			c.wg.Add(1)
			queue <- crawlTask{url: seed, depth: 1}
		}

		// Concurrently fetch robots.txt and sitemap.xml in the background
		c.wg.Add(2)
		go func(s string) {
			defer c.wg.Done()
			c.crawlRobotsTxt(s, queue)
		}(seed)
		go func(s string) {
			defer c.wg.Done()
			c.crawlSitemapXML(s, queue)
		}(seed)
	}

	// Wait for all workers to finish and close the queue
	c.wg.Wait()
	close(queue)
}

func (c *Crawler) getUserAgent() string {
	if c.rotateUA {
		return userAgents[rand.Intn(len(userAgents))]
	}
	return "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/115.0.0.0 Safari/537.36"
}

func (c *Crawler) crawlRobotsTxt(baseURLStr string, queue chan crawlTask) {
	u, err := url.Parse(baseURLStr)
	if err != nil {
		return
	}
	robotsURL := fmt.Sprintf("%s://%s/robots.txt", u.Scheme, u.Host)

	req, err := http.NewRequest("GET", robotsURL, nil)
	if err != nil {
		return
	}
	req.Header.Set("User-Agent", c.getUserAgent())
	for k, v := range c.headers {
		req.Header.Set(k, v)
	}

	resp, err := c.client.Do(req)
	if err != nil {
		if c.verbose {
			fmt.Fprintf(os.Stderr, "[!] [error] robots.txt %s - %v\n", robotsURL, err)
		}
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return
	}

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return
	}

	re := regexp.MustCompile(`(?i)(?:allow|disallow|sitemap)\s*:\s*(\S+)`)
	matches := re.FindAllStringSubmatch(string(bodyBytes), -1)

	for _, m := range matches {
		if len(m) > 1 {
			pathOrURL := m[1]
			resolved := jsparser.ResolveURL(robotsURL, pathOrURL)
			if resolved == "" {
				continue
			}

			if c.checkScope(resolved) {
				c.results <- &output.Result{
					Source: "robots",
					Method: "GET",
					URL:    resolved,
					Depth:  1,
				}
				if c.markVisited(resolved) {
					c.wg.Add(1)
					go func(r string) {
						queue <- crawlTask{url: r, depth: 1}
					}(resolved)
				}
			}
		}
	}
}

func (c *Crawler) crawlSitemapXML(baseURLStr string, queue chan crawlTask) {
	u, err := url.Parse(baseURLStr)
	if err != nil {
		return
	}
	sitemapURL := fmt.Sprintf("%s://%s/sitemap.xml", u.Scheme, u.Host)

	req, err := http.NewRequest("GET", sitemapURL, nil)
	if err != nil {
		return
	}
	req.Header.Set("User-Agent", c.getUserAgent())
	for k, v := range c.headers {
		req.Header.Set(k, v)
	}

	resp, err := c.client.Do(req)
	if err != nil {
		if c.verbose {
			fmt.Fprintf(os.Stderr, "[!] [error] sitemap.xml %s - %v\n", sitemapURL, err)
		}
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return
	}

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return
	}

	re := regexp.MustCompile(`(?i)<loc>\s*([^<]+)\s*</loc>`)
	matches := re.FindAllStringSubmatch(string(bodyBytes), -1)

	for _, m := range matches {
		if len(m) > 1 {
			resolved := strings.TrimSpace(m[1])
			if resolved == "" {
				continue
			}

			if c.checkScope(resolved) {
				c.results <- &output.Result{
					Source: "sitemap",
					Method: "GET",
					URL:    resolved,
					Depth:  1,
				}
				if c.markVisited(resolved) {
					c.wg.Add(1)
					go func(r string) {
						queue <- crawlTask{url: r, depth: 1}
					}(resolved)
				}
			}
		}
	}
}

func (c *Crawler) worker(queue chan crawlTask) {
	for task := range queue {
		if c.throttle != nil {
			<-c.throttle
		} else if c.delay > 0 {
			time.Sleep(c.delay)
		}
		c.process(task, queue)
		c.wg.Done()
	}
}

func (c *Crawler) parseForms(pageURL string, html string) {
	formRegex := regexp.MustCompile(`(?is)<form[^>]*>(.*?)</form>`)
	actionAttrRegex := regexp.MustCompile(`(?i)action\s*=\s*["']([^"']*)["']`)
	inputAttrRegex := regexp.MustCompile(`(?is)<input[^>]*>`)
	nameAttrRegex := regexp.MustCompile(`(?i)name\s*=\s*["']([^"']*)["']`)

	forms := formRegex.FindAllString(html, -1)
	for _, formContent := range forms {
		actionMatch := actionAttrRegex.FindStringSubmatch(formContent)
		action := ""
		if len(actionMatch) > 1 {
			action = actionMatch[1]
		}

		resolvedAction := jsparser.ResolveURL(pageURL, action)
		if resolvedAction == "" {
			continue
		}

		if !c.checkScope(resolvedAction) {
			continue
		}

		inputs := inputAttrRegex.FindAllString(formContent, -1)
		var params []string
		for _, input := range inputs {
			nameMatch := nameAttrRegex.FindStringSubmatch(input)
			if len(nameMatch) > 1 {
				name := nameMatch[1]
				if name != "" {
					params = append(params, name+"=FUZZ")
				}
			}
		}

		c.results <- &output.Result{
			Source: "form",
			Method: "POST",
			URL:    resolvedAction,
			Depth:  1,
		}

		if len(params) > 0 {
			paramURL := resolvedAction
			if strings.Contains(resolvedAction, "?") {
				paramURL += "&" + strings.Join(params, "&")
			} else {
				paramURL += "?" + strings.Join(params, "&")
			}
			c.results <- &output.Result{
				Source: "form-param",
				Method: "GET",
				URL:    paramURL,
				Depth:  1,
			}
		}
	}
}

func (c *Crawler) process(task crawlTask, queue chan crawlTask) {
	req, err := http.NewRequest("GET", task.url, nil)
	if err != nil {
		return
	}

	req.Header.Set("User-Agent", c.getUserAgent())
	for k, v := range c.headers {
		req.Header.Set(k, v)
	}

	resp, err := c.client.Do(req)
	if err != nil {
		if c.verbose {
			fmt.Fprintf(os.Stderr, "[!] [error] %s - %v\n", task.url, err)
		}
		if strings.HasPrefix(task.url, "https://") {
			fallbackURL := "http://" + strings.TrimPrefix(task.url, "https://")
			if c.markVisited(fallbackURL) {
				if c.verbose {
					fmt.Fprintf(os.Stderr, "[*] [fallback] Attempting HTTP fallback for %s\n", task.url)
				}
				c.wg.Add(1)
				go func(r string, d int) {
					queue <- crawlTask{url: r, depth: d}
				}(fallbackURL, task.depth)
			}
		}
		return
	}
	defer resp.Body.Close()

	bodyBytes, err := io.ReadAll(io.LimitReader(resp.Body, 4*1024*1024))
	if err != nil {
		return
	}
	bodyStr := string(bodyBytes)

	contentType := strings.ToLower(resp.Header.Get("Content-Type"))
	isHTML := strings.Contains(contentType, "html")
	isJS := strings.Contains(contentType, "javascript") || strings.HasSuffix(strings.ToLower(req.URL.Path), ".js")

	if isHTML {
		hrefs := hrefRegex.FindAllStringSubmatch(bodyStr, -1)
		for _, m := range hrefs {
			if len(m) > 1 {
				resolved := jsparser.ResolveURL(task.url, m[1])
				if resolved == "" {
					continue
				}

				inScope := c.checkScope(resolved)
				if inScope {
					c.results <- &output.Result{
						Source: "href",
						Method: "GET",
						URL:    resolved,
						Depth:  task.depth,
					}

					if task.depth < c.maxDepth {
						if c.markVisited(resolved) {
							c.wg.Add(1)
							go func(r string, d int) {
								queue <- crawlTask{url: r, depth: d}
							}(resolved, task.depth+1)
						}
					}
				}
			}
		}

		srcs := srcRegex.FindAllStringSubmatch(bodyStr, -1)
		for _, m := range srcs {
			if len(m) > 1 {
				resolved := jsparser.ResolveURL(task.url, m[1])
				if resolved == "" {
					continue
				}

				inScope := c.checkScope(resolved)
				if inScope {
					c.results <- &output.Result{
						Source: "script",
						Method: "GET",
						URL:    resolved,
						Depth:  task.depth,
					}

					if c.jsCrawl {
						c.parseJS(resolved, task.depth)
					}
				}
			}
		}

		c.parseForms(task.url, bodyStr)
	} else if isJS && c.jsCrawl {
		c.parseJS(task.url, task.depth)
	}
}

// parseJS downloads and parses a JS file for endpoints
func (c *Crawler) parseJS(jsURL string, depth int) {
	c.mu.Lock()
	if c.visited[jsURL+"#parsed"] {
		c.mu.Unlock()
		return
	}
	c.visited[jsURL+"#parsed"] = true
	c.mu.Unlock()

	content, err := jsparser.FetchJSContent(c.client, jsURL, c.headers)
	if err != nil {
		return
	}

	endpoints := jsparser.ExtractEndpoints(content)
	for _, ep := range endpoints {
		resolved := jsparser.ResolveURL(jsURL, ep)
		if resolved == "" {
			continue
		}

		if c.checkScope(resolved) {
			c.results <- &output.Result{
				Source: "js-link",
				Method: "GET",
				URL:    resolved,
				Depth:  depth,
			}
		}
	}
}

func (c *Crawler) markVisited(u string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.visited[u] {
		return false
	}
	c.visited[u] = true
	return true
}

func (c *Crawler) checkScope(targetURLStr string) bool {
	u, err := url.Parse(targetURLStr)
	if err != nil {
		return false
	}

	targetHost := u.Hostname()
	targetRoot := getRootDomain(targetHost)

	// If the target's root domain matches any of our seed root domains, it's in scope
	if c.allowedRoots[targetRoot] {
		return c.checkPathScope(u.Path)
	}

	return false
}

func (c *Crawler) checkPathScope(targetPath string) bool {
	if !c.inside {
		return true
	}
	return strings.HasPrefix(targetPath, c.initialPath)
}

// getRootDomain extracts the base registrable domain from a hostname
func getRootDomain(host string) string {
	host = strings.ToLower(host)
	if idx := strings.Index(host, ":"); idx != -1 {
		host = host[:idx]
	}
	parts := strings.Split(host, ".")
	if len(parts) <= 2 {
		return host
	}

	// Common second-level domains (SLDs)
	slds := map[string]bool{
		"co": true, "com": true, "edu": true, "gov": true, "net": true, "org": true, "ac": true,
	}

	prev := parts[len(parts)-2]
	if slds[prev] && len(parts) >= 3 {
		return strings.Join(parts[len(parts)-3:], ".")
	}

	return strings.Join(parts[len(parts)-2:], ".")
}

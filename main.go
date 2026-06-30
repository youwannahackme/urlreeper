package main

import (
	"bufio"
	"context"
	"crypto/tls"
	"flag"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/youwannahackme/urlreeper/pkg/crawler"
	"github.com/youwannahackme/urlreeper/pkg/jsparser"
	"github.com/youwannahackme/urlreeper/pkg/output"
	"github.com/youwannahackme/urlreeper/pkg/passive"
	"github.com/youwannahackme/urlreeper/pkg/validator"
)

// Custom type to support multiple header flags (e.g. -H "Cookie: foo" -H "Auth: bar")
type headerFlags []string

func (h *headerFlags) String() string {
	return strings.Join(*h, ", ")
}

func (h *headerFlags) Set(value string) error {
	*h = append(*h, value)
	return nil
}

const banner = `
██╗   ██╗██████╗ ██╗     ██████╗ ███████╗███████╗██████╗ ███████╗██████╗ 
██║   ██║██╔══██╗██║     ██╔══██╗██╔════╝██╔════╝██╔══██╗██╔════╝██╔══██╗
██║   ██║██████╔╝██║     ██████╔╝█████╗  █████╗  ██████╔╝█████╗  ██████╔╝
██║   ██║██╔══██╗██║     ██╔══██╗██╔══╝  ██╔══╝  ██╔═══╝ ██╔══╝  ██╔══██╗
╚██████╔╝██║  ██║███████╗██║  ██║███████╗███████╗██║     ███████╗██║  ██║
 ╚═════╝ ╚═╝  ╚═╝╚══════╝╚═╝  ╚═╝╚══════╝╚══════╝╚═╝     ╚══════╝╚═╝  ╚═╝

                       v1.0.0 | By @whoami_404
             https://github.com/youwannahackme
`

func main() {
	// 1. Define CLI flags
	mode := flag.String("mode", "all", "Scan mode: crawl (active), passive (archives), js (js parsing), all")

	targetURL := new(string)
	flag.StringVar(targetURL, "u", "", "target url / list to crawl")
	flag.StringVar(targetURL, "list", "", "target url / list to crawl")

	domain := flag.String("domain", "", "Target domain for passive gathering")
	jsFile := flag.String("file", "", "Local JavaScript file to parse")

	threads := flag.Int("t", 20, "Number of concurrent threads")
	timeoutSec := flag.Int("timeout", 10, "Request timeout in seconds")
	proxyURLStr := flag.String("proxy", "", "Proxy URL (e.g. http://127.0.0.1:8080)")
	insecure := flag.Bool("insecure", false, "Skip TLS verification")

	outputFile := new(string)
	flag.StringVar(outputFile, "o", "", "file to write output to")
	flag.StringVar(outputFile, "output", "", "file to write output to")

	jsonOutput := flag.Bool("json", false, "Output results in JSON format")
	verbose := flag.Bool("verbose", false, "Show verbose output (includes depth info)")
	noColor := flag.Bool("no-color", false, "Disable color output in terminal")
	silent := flag.Bool("silent", false, "Silent mode (suppress banner and status info)")
	versionFlag := flag.Bool("version", false, "display project version")

	// Crawl mode flags
	depth := flag.Int("d", 2, "Maximum crawl depth")
	inside := flag.Bool("inside", false, "Only crawl paths inside the initial URL path")

	jsCrawl := new(bool)
	flag.BoolVar(jsCrawl, "js-crawl", true, "Enable endpoint extraction from discovered JS files")
	flag.BoolVar(jsCrawl, "jc", true, "Enable endpoint extraction from discovered JS files")

	var headers headerFlags
	flag.Var(&headers, "H", "Custom headers (e.g. -H \"Cookie: session=123\")")

	// Passive mode flags
	placeholder := flag.String("placeholder", "FUZZ", "Placeholder for parameter values in passive mode")
	clean := flag.Bool("clean", true, "Clean and deduplicate passive URLs (ParamSpider style)")
	paramsOnly := flag.Bool("params-only", false, "Only output URLs with parameters in passive mode")

	// Regex matching flags
	var matchRegexRaw string
	flag.StringVar(&matchRegexRaw, "mr", "", "regex or list of regex to match on output url (cli, file)")
	flag.StringVar(&matchRegexRaw, "match-regex", "", "regex or list of regex to match on output url (cli, file)")

	// New flags
	delayMs := flag.Int("delay", 0, "Delay between requests per thread in milliseconds")
	rateLimit := flag.Int("rate-limit", 0, "Global rate limit in Requests Per Second")
	rotateUA := flag.Bool("rotate-ua", false, "Enable random User-Agent rotation")
	live := flag.Bool("live", false, "Validate found URLs by checking if they are alive")
	statusFilterRaw := flag.String("status-filter", "", "Comma-separated list of status codes to output when -live is enabled")

	// Define custom help menu
	flag.Usage = func() {
		fmt.Fprint(os.Stderr, banner)
		fmt.Fprintf(os.Stderr, "Usage of urlreeper:\n\n")
		fmt.Fprintf(os.Stderr, "Target Options:\n")
		fmt.Fprintf(os.Stderr, "  -u, -list string          target url / list to crawl\n")
		fmt.Fprintf(os.Stderr, "  -domain string            Target domain for passive gathering\n")
		fmt.Fprintf(os.Stderr, "  -file string              Local JavaScript file to parse\n\n")

		fmt.Fprintf(os.Stderr, "Scan Options:\n")
		fmt.Fprintf(os.Stderr, "  -mode string              Scan mode: crawl (active), passive (archives), js (js parsing), all (default \"all\")\n")
		fmt.Fprintf(os.Stderr, "  -d int                    Maximum crawl depth (default 2)\n")
		fmt.Fprintf(os.Stderr, "  -inside                   Only crawl paths inside the initial URL path\n")
		fmt.Fprintf(os.Stderr, "  -jc, -js-crawl            Enable endpoint extraction from discovered JS files (default true)\n")
		fmt.Fprintf(os.Stderr, "  -H value                  Custom headers (e.g. -H \"Cookie: session=123\")\n\n")

		fmt.Fprintf(os.Stderr, "Passive Options:\n")
		fmt.Fprintf(os.Stderr, "  -placeholder string       Placeholder for parameter values in passive mode (default \"FUZZ\")\n")
		fmt.Fprintf(os.Stderr, "  -clean                    Clean and deduplicate passive URLs (ParamSpider style) (default true)\n")
		fmt.Fprintf(os.Stderr, "  -params-only              Only output URLs with parameters in passive mode\n\n")

		fmt.Fprintf(os.Stderr, "Filter Options:\n")
		fmt.Fprintf(os.Stderr, "  -mr, -match-regex string  regex or list of regex to match on output url (cli, file)\n\n")

		fmt.Fprintf(os.Stderr, "Performance Options:\n")
		fmt.Fprintf(os.Stderr, "  -t int                    Number of concurrent threads (default 20)\n")
		fmt.Fprintf(os.Stderr, "  -timeout int              Request timeout in seconds (default 10)\n")
		fmt.Fprintf(os.Stderr, "  -proxy string             Proxy URL (e.g. http://127.0.0.1:8080)\n")
		fmt.Fprintf(os.Stderr, "  -insecure                 Skip TLS verification\n")
		fmt.Fprintf(os.Stderr, "  -delay int                Delay between requests per thread in milliseconds (default 0)\n")
		fmt.Fprintf(os.Stderr, "  -rate-limit int           Global rate limit in Requests Per Second (default 0)\n")
		fmt.Fprintf(os.Stderr, "  -rotate-ua                Enable random User-Agent rotation\n\n")

		fmt.Fprintf(os.Stderr, "Live Validation Options:\n")
		fmt.Fprintf(os.Stderr, "  -live                     Validate found URLs by checking if they are alive\n")
		fmt.Fprintf(os.Stderr, "  -status-filter string     Comma-separated list of status codes to output when -live is enabled\n\n")

		fmt.Fprintf(os.Stderr, "Output Options:\n")
		fmt.Fprintf(os.Stderr, "  -o, -output string        file to write output to\n")
		fmt.Fprintf(os.Stderr, "  -json                     Output results in JSON format\n")
		fmt.Fprintf(os.Stderr, "  -verbose                  Show verbose output (includes depth info)\n")
		fmt.Fprintf(os.Stderr, "  -no-color                 Disable color output in terminal\n")
		fmt.Fprintf(os.Stderr, "  -silent                   Silent mode (suppress banner and status info)\n")
		fmt.Fprintf(os.Stderr, "  -version                  display project version\n")
	}

	flag.Parse()

	// Handle version flag
	if *versionFlag {
		fmt.Println("urlreeper version v1.0.0")
		os.Exit(0)
	}

	// Scoping is now automatically determined from the root domains of targets

	// Handle positional arguments as target if targetURL and domain are not set
	if flag.NArg() > 0 && *targetURL == "" && *domain == "" {
		target := flag.Arg(0)
		if strings.HasPrefix(target, "http://") || strings.HasPrefix(target, "https://") {
			*targetURL = target
			u, err := url.Parse(target)
			if err == nil {
				*domain = u.Hostname()
			}
		} else {
			*domain = target
			*targetURL = "https://" + target
		}
	}

	// Compile match regexes if specified
	var matchRegexes []*regexp.Regexp
	if matchRegexRaw != "" {
		var patterns []string
		if _, err := os.Stat(matchRegexRaw); err == nil {
			filePatterns, err := readLines(matchRegexRaw)
			if err == nil {
				patterns = append(patterns, filePatterns...)
			}
		} else {
			parts := strings.Split(matchRegexRaw, ",")
			for _, part := range parts {
				part = strings.TrimSpace(part)
				if part != "" {
					patterns = append(patterns, part)
				}
			}
			// If split failed or empty, use the raw string directly
			if len(patterns) == 0 {
				patterns = append(patterns, matchRegexRaw)
			}
		}

		for _, pattern := range patterns {
			re, err := regexp.Compile(pattern)
			if err != nil {
				fmt.Fprintf(os.Stderr, "[!] Invalid regex pattern %q: %v\n", pattern, err)
				os.Exit(1)
			}
			matchRegexes = append(matchRegexes, re)
		}
	}

	// 2. Print Banner
	if !*silent {
		fmt.Fprint(os.Stderr, banner)
	}

	// 3. Setup HTTP Client
	timeout := time.Duration(*timeoutSec) * time.Second
	client := getHTTPClient(timeout, *insecure, *proxyURLStr)

	// 4. Setup Output Writer
	writer, err := output.NewWriter(*outputFile, *jsonOutput, *verbose, *noColor)
	if err != nil {
		fmt.Fprintf(os.Stderr, "[!] Error initializing output writer: %v\n", err)
		os.Exit(1)
	}
	defer writer.Close()

	// Parse headers map
	headerMap := parseHeaders(headers)

	// Parse status filter if specified
	var statusFilter []int
	if *statusFilterRaw != "" {
		parts := strings.Split(*statusFilterRaw, ",")
		for _, part := range parts {
			part = strings.TrimSpace(part)
			var code int
			_, err := fmt.Sscanf(part, "%d", &code)
			if err == nil {
				statusFilter = append(statusFilter, code)
			}
		}
	}

	ctx := context.Background()
	resultsChan := make(chan *output.Result, 1000)

	var targetResultsChan chan *output.Result
	var val *validator.Validator

	if *live {
		rawResultsChan := make(chan *output.Result, 1000)
		targetResultsChan = rawResultsChan
		val = validator.NewValidator(timeout, *insecure, *proxyURLStr, *threads, statusFilter, rawResultsChan, resultsChan)
		val.Start()
	} else {
		targetResultsChan = resultsChan
	}

	// Goroutine to consume results from channel, filter by regex, and write them
	go func() {
		for res := range resultsChan {
			if len(matchRegexes) > 0 {
				matched := false
				for _, re := range matchRegexes {
					if re.MatchString(res.URL) {
						matched = true
						break
					}
				}
				if !matched {
					continue // Skip if it doesn't match any regex
				}
			}
			writer.Write(res)
		}
	}()

	// 5. Execute Mode
	switch strings.ToLower(*mode) {
	case "crawl":
		seeds := getSeeds(*targetURL)
		if len(seeds) == 0 {
			fmt.Fprintln(os.Stderr, "[!] No target URLs provided for crawling. Use -u or pipe via stdin.")
			os.Exit(1)
		}
		if !*silent {
			fmt.Fprintf(os.Stderr, "[*] Starting active crawl against %d seeds...\n", len(seeds))
		}

		// Build allowed root domains from initial seeds
		allowedRoots := make(map[string]bool)
		for _, seed := range seeds {
			u, err := url.Parse(seed)
			if err == nil {
				root := getRootDomain(u.Hostname())
				if root != "" {
					allowedRoots[root] = true
				}
			}
		}

		c := crawler.NewCrawler(client, *depth, *inside, *jsCrawl, headerMap, *threads, allowedRoots, targetResultsChan, *verbose, time.Duration(*delayMs)*time.Millisecond, *rateLimit, *rotateUA)
		c.Crawl(seeds)

	case "passive":
		var domains []string
		if *domain != "" {
			domains = append(domains, *domain)
		} else {
			seeds := getSeeds(*targetURL)
			for _, seed := range seeds {
				u, err := url.Parse(seed)
				if err == nil {
					domains = append(domains, u.Hostname())
				}
			}
		}

		if len(domains) == 0 {
			fmt.Fprintln(os.Stderr, "[!] No domain provided for passive gathering. Use -domain, -u, or stdin.")
			os.Exit(1)
		}

		// Gather passive URLs for all unique root domains
		uniqueRoots := make(map[string]bool)
		for _, dom := range domains {
			root := getRootDomain(dom)
			if root != "" {
				uniqueRoots[root] = true
			}
		}

		for root := range uniqueRoots {
			if !*silent {
				fmt.Fprintf(os.Stderr, "[*] Gathering passive URLs for domain: %s...\n", root)
			}
			urls := passive.FetchPassiveURLs(ctx, client, root, true)
			if *clean {
				urls = passive.CleanAndProcessURLs(urls, *placeholder, *paramsOnly)
			}
			for _, u := range urls {
				targetResultsChan <- &output.Result{
					Source: "passive",
					URL:    u,
				}
			}
		}

	case "js":
		if *jsFile != "" {
			// Parse local file
			if !*silent {
				fmt.Fprintf(os.Stderr, "[*] Parsing local JS file: %s...\n", *jsFile)
			}
			contentBytes, err := os.ReadFile(*jsFile)
			if err != nil {
				fmt.Fprintf(os.Stderr, "[!] Error reading file: %v\n", err)
				os.Exit(1)
			}
			endpoints := jsparser.ExtractEndpoints(string(contentBytes))
			for _, ep := range endpoints {
				targetResultsChan <- &output.Result{
					Source: "js-file",
					URL:    ep,
				}
			}
		} else {
			// Parse remote URL(s)
			seeds := getSeeds(*targetURL)
			if len(seeds) == 0 {
				fmt.Fprintln(os.Stderr, "[!] No target JS URLs provided. Use -u, -file, or stdin.")
				os.Exit(1)
			}
			for _, seed := range seeds {
				if !*silent {
					fmt.Fprintf(os.Stderr, "[*] Fetching and parsing remote JS: %s...\n", seed)
				}
				content, err := jsparser.FetchJSContent(client, seed, headerMap)
				if err != nil {
					fmt.Fprintf(os.Stderr, "[!] Error fetching %s: %v\n", seed, err)
					continue
				}
				endpoints := jsparser.ExtractEndpoints(content)
				for _, ep := range endpoints {
					resolved := jsparser.ResolveURL(seed, ep)
					if resolved != "" {
						targetResultsChan <- &output.Result{
							Source: "js-link",
							URL:    resolved,
						}
					}
				}
			}
		}

	case "all":
		// Run passive, then active crawl using all passive URLs as seeds
		seeds := getSeeds(*targetURL)
		if len(seeds) == 0 {
			fmt.Fprintln(os.Stderr, "[!] No target URLs provided. Use -u, a positional argument, or stdin.")
			os.Exit(1)
		}

		// Gather passive URLs for all unique root domains of the seeds
		uniqueRoots := make(map[string]bool)
		for _, seed := range seeds {
			u, err := url.Parse(seed)
			if err == nil {
				root := getRootDomain(u.Hostname())
				if root != "" {
					uniqueRoots[root] = true
				}
			}
		}

		var passiveSeeds []string
		for root := range uniqueRoots {
			if !*silent {
				fmt.Fprintf(os.Stderr, "[*] Step 1: Gathering passive URLs for domain: %s...\n", root)
			}
			urls := passive.FetchPassiveURLs(ctx, client, root, true)
			if *clean {
				urls = passive.CleanAndProcessURLs(urls, *placeholder, *paramsOnly)
			}
			passiveSeeds = append(passiveSeeds, urls...)
			for _, u := range urls {
				targetResultsChan <- &output.Result{
					Source: "passive",
					URL:    u,
				}
			}
		}

		// Combine initial seeds and passive seeds (filtering passive seeds to ensure they match allowed roots)
		allSeeds := append([]string{}, seeds...)
		for _, ps := range passiveSeeds {
			u, err := url.Parse(ps)
			if err == nil {
				root := getRootDomain(u.Hostname())
				if uniqueRoots[root] {
					allSeeds = append(allSeeds, ps)
				}
			}
		}

		// Deduplicate seeds to keep it clean
		uniqueSeeds := make(map[string]bool)
		var finalSeeds []string
		for _, s := range allSeeds {
			if !uniqueSeeds[s] {
				uniqueSeeds[s] = true
				finalSeeds = append(finalSeeds, s)
			}
		}

		if !*silent {
			fmt.Fprintf(os.Stderr, "[*] Step 2: Starting active crawl on %d unique seeds...\n", len(finalSeeds))
		}
		c := crawler.NewCrawler(client, *depth, *inside, *jsCrawl, headerMap, *threads, uniqueRoots, targetResultsChan, *verbose, time.Duration(*delayMs)*time.Millisecond, *rateLimit, *rotateUA)
		c.Crawl(finalSeeds)

	default:
		fmt.Fprintf(os.Stderr, "[!] Unknown mode: %s. Supported: crawl, passive, js, all\n", *mode)
		os.Exit(1)
	}

	// Close the target results channel and wait for validation/writing to finish
	close(targetResultsChan)
	if *live && val != nil {
		val.Wait()
	}
	// Add a tiny delay to let the writer goroutine flush completely
	time.Sleep(100 * time.Millisecond)
	if !*silent {
		fmt.Fprintln(os.Stderr, "[*] Done!")
	}
}

func getHTTPClient(timeout time.Duration, insecure bool, proxyURLStr string) *http.Client {
	transport := &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: insecure},
	}

	if proxyURLStr != "" {
		proxyURL, err := url.Parse(proxyURLStr)
		if err == nil {
			transport.Proxy = http.ProxyURL(proxyURL)
		} else {
			fmt.Fprintf(os.Stderr, "[!] Error parsing proxy URL: %v\n", err)
		}
	}

	return &http.Client{
		Transport: transport,
		Timeout:   timeout,
	}
}

func parseHeaders(flags headerFlags) map[string]string {
	headers := make(map[string]string)
	for _, h := range flags {
		parts := strings.SplitN(h, ":", 2)
		if len(parts) == 2 {
			headers[strings.TrimSpace(parts[0])] = strings.TrimSpace(parts[1])
		}
	}
	return headers
}

func getSeeds(targetURLFlag string) []string {
	var seeds []string

	// 1. Check stdin
	stdinLines := readStdin()
	if len(stdinLines) > 0 {
		seeds = append(seeds, stdinLines...)
	}

	// 2. Check flag
	if targetURLFlag != "" {
		// Check if it's a file
		if _, err := os.Stat(targetURLFlag); err == nil {
			fileSeeds, err := readLines(targetURLFlag)
			if err == nil {
				seeds = append(seeds, fileSeeds...)
			}
		} else {
			// Comma separated
			parts := strings.Split(targetURLFlag, ",")
			for _, part := range parts {
				part = strings.TrimSpace(part)
				if part != "" {
					seeds = append(seeds, part)
				}
			}
		}
	}

	// Normalize: ensure all seeds have a scheme (default to https://)
	for i, seed := range seeds {
		if !strings.HasPrefix(seed, "http://") && !strings.HasPrefix(seed, "https://") {
			seeds[i] = "https://" + seed
		}
	}

	return seeds
}

func readLines(path string) ([]string, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var lines []string
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line != "" {
			lines = append(lines, line)
		}
	}
	return lines, scanner.Err()
}

func readStdin() []string {
	var lines []string
	stat, _ := os.Stdin.Stat()
	if (stat.Mode() & os.ModeCharDevice) == 0 {
		scanner := bufio.NewScanner(os.Stdin)
		for scanner.Scan() {
			line := strings.TrimSpace(scanner.Text())
			if line != "" {
				lines = append(lines, line)
			}
		}
		if err := scanner.Err(); err != nil {
			fmt.Fprintf(os.Stderr, "[!] Error reading stdin: %v\n", err)
		}
	}
	return lines
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

package validator

import (
	"crypto/tls"
	"net/http"
	"net/url"
	"sync"
	"time"
	"github.com/youwannahackme/urlreeper/pkg/output"
)

type Validator struct {
	client     *http.Client
	threads    int
	filters    map[int]bool
	inputChan  <-chan *output.Result
	outputChan chan<- *output.Result
	wg         sync.WaitGroup
}

func NewValidator(timeout time.Duration, insecure bool, proxyURLStr string, threads int, statusFilter []int, input <-chan *output.Result, output chan<- *output.Result) *Validator {
	transport := &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: insecure},
	}
	if proxyURLStr != "" {
		proxyURL, err := url.Parse(proxyURLStr)
		if err == nil {
			transport.Proxy = http.ProxyURL(proxyURL)
		}
	}
	client := &http.Client{
		Transport: transport,
		Timeout:   timeout,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 10 {
				return http.ErrUseLastResponse
			}
			return nil
		},
	}

	filterMap := make(map[int]bool)
	for _, code := range statusFilter {
		filterMap[code] = true
	}

	return &Validator{
		client:     client,
		threads:    threads,
		filters:    filterMap,
		inputChan:  input,
		outputChan: output,
	}
}

func (v *Validator) Start() {
	for i := 0; i < v.threads; i++ {
		v.wg.Add(1)
		go v.worker()
	}
}

func (v *Validator) Wait() {
	v.wg.Wait()
	close(v.outputChan)
}

func (v *Validator) worker() {
	defer v.wg.Done()
	for res := range v.inputChan {
		req, err := http.NewRequest("GET", res.URL, nil)
		if err != nil {
			continue
		}
		req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/115.0.0.0 Safari/537.36")

		resp, err := v.client.Do(req)
		if err != nil {
			continue
		}
		resp.Body.Close()

		res.StatusCode = resp.StatusCode

		if len(v.filters) > 0 && !v.filters[res.StatusCode] {
			continue
		}

		v.outputChan <- res
	}
}

package captcha

import (
	"testing"

	"github.com/go-rod/rod/lib/proto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIntegration_DetectRealCaptchaPages(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	browser := setupBrowser(t)

	tests := []struct {
		name         string
		url          string
		wantProvider Provider
	}{
		{
			name:         "google recaptcha v2 demo",
			url:          "https://www.google.com/recaptcha/api2/demo",
			wantProvider: ProviderRecaptchaV2,
		},
		{
			name:         "hcaptcha official demo",
			url:          "https://accounts.hcaptcha.com/demo",
			wantProvider: ProviderHCaptcha,
		},
		{
			name:         "2captcha turnstile demo",
			url:          "https://2captcha.com/demo/cloudflare-turnstile",
			wantProvider: ProviderTurnstile,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			page, err := browser.Page(proto.TargetCreateTarget{URL: tt.url})
			if err != nil {
				t.Skipf("network navigation to %s failed: %v", tt.url, err)
				return
			}
			defer func() { _ = page.Close() }()
			if err := page.WaitLoad(); err != nil {
				t.Skipf("page load for %s failed: %v", tt.url, err)
				return
			}

			info, err := Identify(page)
			require.NoError(t, err)
			require.NotNil(t, info, "expected captcha to be detected on %s", tt.url)

			t.Logf("detected: provider=%s sitekey=%s url=%s", info.Provider, info.SiteKey, info.PageURL)

			assert.Equal(t, tt.wantProvider, info.Provider)
			assert.NotEmpty(t, info.SiteKey)
		})
	}
}

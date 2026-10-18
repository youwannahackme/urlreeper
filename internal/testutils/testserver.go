package testutils

import (
	"fmt"
	"net"
	"net/http"
)

// TestServer is a local HTTP server for functional tests.
type TestServer struct {
	mux      *http.ServeMux
	listener net.Listener
	URL      string
}

// NewTestServer starts a local HTTP server on a random port with
// a small site structure suitable for crawl testing.
func NewTestServer() (*TestServer, error) {
	mux := http.NewServeMux()

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, fmt.Errorf("failed to start test server: %w", err)
	}
	baseURL := fmt.Sprintf("http://127.0.0.1:%d", listener.Addr().(*net.TCPAddr).Port)

	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/html")
		_, _ = fmt.Fprintf(w, `<!DOCTYPE html>
<html>
<head><title>Test Home</title></head>
<body>
	<a href="%s/about">About</a>
	<a href="%s/contact">Contact</a>
	<a href="%s/blog">Blog</a>
	<script src="%s/static/app.js"></script>
</body>
</html>`, baseURL, baseURL, baseURL, baseURL)
	})

	mux.HandleFunc("/about", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = fmt.Fprintf(w, `<!DOCTYPE html>
<html>
<head><title>About</title></head>
<body>
	<a href="%s/">Home</a>
	<a href="%s/team">Team</a>
</body>
</html>`, baseURL, baseURL)
	})

	mux.HandleFunc("/contact", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = fmt.Fprintf(w, `<!DOCTYPE html>
<html>
<head><title>Contact</title></head>
<body>
	<a href="%s/">Home</a>
	<form action="%s/submit" method="POST">
		<input type="email" name="email">
		<input type="submit" value="Send">
	</form>
</body>
</html>`, baseURL, baseURL)
	})

	mux.HandleFunc("/blog", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = fmt.Fprintf(w, `<!DOCTYPE html>
<html>
<head><title>Blog</title></head>
<body>
	<a href="%s/">Home</a>
	<a href="%s/blog/post-1">First Post</a>
	<a href="%s/blog/post-2">Second Post</a>
</body>
</html>`, baseURL, baseURL, baseURL)
	})

	mux.HandleFunc("/blog/post-1", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = fmt.Fprintf(w, `<!DOCTYPE html>
<html><head><title>Post 1</title></head>
<body><a href="%s/blog">Back to Blog</a></body>
</html>`, baseURL)
	})

	mux.HandleFunc("/blog/post-2", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = fmt.Fprintf(w, `<!DOCTYPE html>
<html><head><title>Post 2</title></head>
<body><a href="%s/blog">Back to Blog</a></body>
</html>`, baseURL)
	})

	mux.HandleFunc("/team", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = fmt.Fprintf(w, `<!DOCTYPE html>
<html><head><title>Team</title></head>
<body><a href="%s/about">About</a></body>
</html>`, baseURL)
	})

	mux.HandleFunc("/submit", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = fmt.Fprint(w, `<!DOCTYPE html><html><head><title>Thanks</title></head><body>OK</body></html>`)
	})

	mux.HandleFunc("/static/app.js", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/javascript")
		_, _ = fmt.Fprintf(w, `// app.js
var awsKey = "AKIAIOSFODNN7EXAMPLE";
fetch("%s/api/v1/users").then(r => r.json());
fetch("%s/api/v2/items").then(r => r.json());
`, baseURL, baseURL)
	})

	mux.HandleFunc("/api/v1/users", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `[{"id":1,"name":"Alice"}]`)
	})

	mux.HandleFunc("/api/v2/items", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `[{"id":100,"item":"widget"}]`)
	})

	mux.HandleFunc("/robots.txt", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		_, _ = fmt.Fprintf(w, "User-agent: *\nDisallow: %s/secret-admin\n", baseURL)
	})

	mux.HandleFunc("/secret-admin", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = fmt.Fprint(w, `<!DOCTYPE html><html><body>Admin Panel</body></html>`)
	})

	mux.HandleFunc("/sitemap.xml", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/xml")
		_, _ = fmt.Fprintf(w, `<?xml version="1.0" encoding="UTF-8"?>
<urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">
  <url><loc>%s/sitemap-target</loc></url>
</urlset>`, baseURL)
	})

	mux.HandleFunc("/sitemap-target", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = fmt.Fprint(w, `<!DOCTYPE html><html><body>Sitemap Target</body></html>`)
	})

	mux.HandleFunc("/redirect-302", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, baseURL+"/destination", http.StatusFound)
	})

	mux.HandleFunc("/destination", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = fmt.Fprint(w, `<!DOCTYPE html><html><body>Redirect Destination</body></html>`)
	})

	mux.HandleFunc("/a/b/c/d/index.html", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = fmt.Fprint(w, `<!DOCTYPE html><html><body>Deep nested page</body></html>`)
	})

	// Similar URLs
	for i := 1; i <= 15; i++ {
		path := fmt.Sprintf("/users/%d", i)
		mux.HandleFunc(path, func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/html")
			_, _ = fmt.Fprintf(w, `<!DOCTYPE html><html><body>User Profile</body></html>`)
		})
	}

	mux.HandleFunc("/items", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = fmt.Fprintf(w, `<!DOCTYPE html><html><body>Items list: %s</body></html>`, r.URL.RawQuery)
	})

	mux.HandleFunc("/files/test.pdf", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/pdf")
		_, _ = fmt.Fprint(w, `%PDF-1.4 test file`)
	})

	mux.HandleFunc("/files/test.png", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		_, _ = fmt.Fprint(w, `\x89PNG test image`)
	})

	mux.HandleFunc("/tech", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.Header().Set("X-Powered-By", "Express")
		w.Header().Set("Server", "nginx/1.18.0")
		_, _ = fmt.Fprint(w, `<!DOCTYPE html><html><body>Tech detection page</body></html>`)
	})

	mux.HandleFunc("/echo-headers", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		customHeader := r.Header.Get("X-Custom-Test")
		_, _ = fmt.Fprintf(w, `{"X-Custom-Test":"%s"}`, customHeader)
	})

	mux.HandleFunc("/rich-form", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = fmt.Fprintf(w, `<!DOCTYPE html>
<html>
<body>
	<form action="%s/form-submit" method="POST">
		<input type="text" name="username" value="admin">
		<input type="password" name="password">
		<input type="tel" name="phone">
		<select name="role">
			<option value="user">User</option>
			<option value="admin">Admin</option>
		</select>
		<textarea name="bio">My Bio</textarea>
		<input type="submit" value="Submit">
	</form>
</body>
</html>`, baseURL)
	})

	mux.HandleFunc("/form-submit", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = fmt.Fprint(w, `<!DOCTYPE html><html><body>Form processed</body></html>`)
	})

	go func() {
		_ = http.Serve(listener, mux)
	}()

	return &TestServer{
		mux:      mux,
		listener: listener,
		URL:      baseURL,
	}, nil
}

// Close shuts down the test server.
func (ts *TestServer) Close() error {
	return ts.listener.Close()
}

package integrate

import (
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"strings"
)

// recordingRT captures the Cookie header seen at the transport layer while
// delegating the actual request to http.DefaultTransport.
type recordingRT struct {
	sawCookie string
}

func (r *recordingRT) RoundTrip(req *http.Request) (*http.Response, error) {
	if req != nil {
		r.sawCookie = req.Header.Get("Cookie")
	}
	return http.DefaultTransport.RoundTrip(req)
}

func TestNewAuthRoundTripper_CookiePropagation(t *testing.T) {
	// Arrange a server that reports the Cookie header it received
	cookieCh := make(chan string, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cookieCh <- r.Header.Get("Cookie")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	}))
	defer srv.Close()

	// Build a cookie jar with a pre-set cookie for the server URL
	jar, _ := cookiejar.New(nil)
	u, _ := url.Parse(srv.URL)
	jar.SetCookies(u, []*http.Cookie{{Name: "sid", Value: "xyz", Path: "/", Expires: time.Now().Add(time.Hour)}})

	// Build auth RoundTripper with cookie jar and default transport
	rt, err := NewAuthRoundTripper(jar, http.DefaultTransport, 0)
	assert.EqualValues(t, nil, err)

	// Act: perform a simple request via the auth RoundTripper
	req, _ := http.NewRequest(http.MethodGet, srv.URL, nil)
	resp, err := rt.RoundTrip(req)
	assert.EqualValues(t, nil, err)
	assert.EqualValues(t, http.StatusOK, resp.StatusCode)

	// Assert: server observed the cookie from the jar
	got := <-cookieCh
	assert.EqualValues(t, true, len(got) > 0, "expected Cookie header to be present")
	assert.EqualValues(t, true, containsCookie(got, "sid", "xyz"))
}

func TestNewAuthRoundTripper_WrapBaseTransport_AddsCookies(t *testing.T) {
	// Arrange a server that always 200s
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	}))
	defer srv.Close()

	// Prepare jar with a cookie for the server
	jar, _ := cookiejar.New(nil)
	u, _ := url.Parse(srv.URL)
	jar.SetCookies(u, []*http.Cookie{{Name: "token", Value: "abc", Path: "/", Expires: time.Now().Add(time.Hour)}})

	// recording base transport should see Cookie header due to cookie wrap
	rec := &recordingRT{}
	rt, err := NewAuthRoundTripper(jar, rec, 0)
	assert.EqualValues(t, nil, err)

	// Act
	req, _ := http.NewRequest(http.MethodGet, srv.URL, nil)
	resp, err := rt.RoundTrip(req)
	assert.EqualValues(t, nil, err)
	assert.EqualValues(t, http.StatusOK, resp.StatusCode)

	// Assert base transport observed cookie header
	assert.EqualValues(t, true, len(rec.sawCookie) > 0, "expected Cookie header at base transport")
	assert.EqualValues(t, true, containsCookie(rec.sawCookie, "token", "abc"))
}

// containsCookie checks header string contains name=value pair.
func containsCookie(header, name, value string) bool {
	return strings.Contains(header, name+"="+value)
}

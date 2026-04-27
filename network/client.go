package network

import (
	"bytes"
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/cookiejar"
	"strings"
	"time"
)

const (
	userAgent     = "CCTP/android64_vpn/2093"
	requestAccept = "text/html,text/xml,application/xhtml+xml,application/x-javascript,*/*"
)

// StateProvider provides the global state values needed for requests.
type StateProvider interface {
	GetClientID() string
	GetAlgoID() string
	GetSchoolID() string
	GetDomain() string
	GetArea() string
	SetArea(string)
	SetSchoolID(string)
	SetDomain(string)
}

// redirectInterceptor is an http.RoundTripper that handles redirects with custom headers.
type redirectInterceptor struct {
	inner    http.RoundTripper
	state    StateProvider
	maxRedir int
}

func (r *redirectInterceptor) RoundTrip(req *http.Request) (*http.Response, error) {
	resp, err := r.inner.RoundTrip(req)
	if err != nil {
		log.Printf("[Redirect] RoundTrip error for %s %s: %v", req.Method, req.URL.String(), err)
		return resp, err
	}

	redirectCount := 0
	for isRedirect(resp.StatusCode) && redirectCount < r.maxRedir {
		redirectCount++

		// Extract routing headers (check both CDC-* and plain variants)
		area := getCDCHeader(resp, "Area")
		if area != "" {
			log.Printf("[Redirect] Header area=%s", area)
			r.state.SetArea(area)
		}
		schoolID := getCDCHeader(resp, "SchoolId")
		if schoolID != "" {
			log.Printf("[Redirect] Header schoolid=%s", schoolID)
			r.state.SetSchoolID(schoolID)
		}
		domain := getCDCHeader(resp, "Domain")
		if domain != "" {
			log.Printf("[Redirect] Header domain=%s", domain)
			r.state.SetDomain(domain)
		}

		location := resp.Header.Get("Location")
		log.Printf("[Redirect] #%d %d -> %s", redirectCount, resp.StatusCode, location)
		if location == "" {
			log.Println("[Redirect] Empty Location header, stopping redirect chain")
			break
		}

		nextURL, err := req.URL.Parse(location)
		if err != nil {
			resp.Body.Close()
			return nil, err
		}

		// Close old response body
		resp.Body.Close()

		var newBody io.ReadCloser
		if req.Body != nil {
			newBody, err = cloneRequestBody(req)
			if err != nil {
				return nil, err
			}
		}

		newReq, err := http.NewRequestWithContext(req.Context(), req.Method, nextURL.String(), newBody)
		if err != nil {
			if newBody != nil {
				newBody.Close()
			}
			return nil, err
		}
		newReq.GetBody = req.GetBody
		newReq.ContentLength = req.ContentLength
		// Copy headers
		for k, v := range req.Header {
			newReq.Header[k] = v
		}
		// Add routing headers if not present
		if r.state.GetSchoolID() != "" && newReq.Header.Get("CDC-SchoolId") == "" {
			newReq.Header.Set("CDC-SchoolId", r.state.GetSchoolID())
		}
		if r.state.GetDomain() != "" && newReq.Header.Get("CDC-Domain") == "" {
			newReq.Header.Set("CDC-Domain", r.state.GetDomain())
		}
		if r.state.GetArea() != "" && newReq.Header.Get("CDC-Area") == "" {
			newReq.Header.Set("CDC-Area", r.state.GetArea())
		}

		resp, err = r.inner.RoundTrip(newReq)
		if err != nil {
			return nil, err
		}
		req = newReq
	}
	return resp, nil
}

// cloneRequestBody creates a fresh body reader for redirected requests.
func cloneRequestBody(req *http.Request) (io.ReadCloser, error) {
	if req.Body == nil {
		return nil, nil
	}
	if req.GetBody != nil {
		return req.GetBody()
	}

	buf, err := io.ReadAll(req.Body)
	if err != nil {
		return nil, err
	}

	req.Body = io.NopCloser(bytes.NewReader(buf))
	req.GetBody = func() (io.ReadCloser, error) {
		return io.NopCloser(bytes.NewReader(buf)), nil
	}
	req.ContentLength = int64(len(buf))
	return req.GetBody()
}

func isRedirect(code int) bool {
	return code == 301 || code == 302 || code == 303 || code == 307 || code == 308
}

// NewHTTPClient creates a configured HTTP client with redirect handling.
func NewHTTPClient(state StateProvider) *http.Client {
	transport := &redirectInterceptor{
		inner:    http.DefaultTransport,
		state:    state,
		maxRedir: 5,
	}
	jar, err := cookiejar.New(nil)
	if err != nil {
		log.Printf("[HTTPClient] Failed to create cookie jar: %v", err)
	}
	return &http.Client{
		Transport: transport,
		Timeout:   10 * time.Second,
		Jar:       jar,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
}

// Post sends a POST request with authentication headers.
func Post(client *http.Client, url, data string, state StateProvider, extraHeaders map[string]string) (string, error) {
	req, err := http.NewRequest("POST", url, strings.NewReader(data))
	if err != nil {
		log.Printf("[Post] Failed to create request for %s: %v", url, err)
		return "", fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Accept", requestAccept)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	// MD5 checksum
	hash := md5.Sum([]byte(data))
	req.Header.Set("CDC-Checksum", hex.EncodeToString(hash[:]))
	req.Header.Set("Client-ID", state.GetClientID())
	req.Header.Set("Algo-ID", state.GetAlgoID())

	if v := state.GetSchoolID(); v != "" {
		req.Header.Set("CDC-SchoolId", v)
	}
	if v := state.GetDomain(); v != "" {
		req.Header.Set("CDC-Domain", v)
	}
	if v := state.GetArea(); v != "" {
		req.Header.Set("CDC-Area", v)
	}

	for k, v := range extraHeaders {
		req.Header.Set(k, v)
	}

	resp, err := client.Do(req)
	if err != nil {
		log.Printf("[Post] Request to %s failed: %v", url, err)
		return "", fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Printf("[Post] Failed to read body from %s: %v", url, err)
		return "", fmt.Errorf("read body: %w", err)
	}
	if len(body) == 0 {
		finalURL := ""
		if resp.Request != nil && resp.Request.URL != nil {
			finalURL = resp.Request.URL.String()
		}
		log.Printf("[Post] Empty response body from %s (status=%d, finalURL=%s)", url, resp.StatusCode, finalURL)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		log.Printf("[Post] %s returned %d: %s", url, resp.StatusCode, string(body))
	}

	return string(body), nil
}

// PostRaw sends a POST request and returns raw bytes (for binary responses like ZSM).
func PostRaw(client *http.Client, url, data string, state StateProvider) ([]byte, error) {
	req, err := http.NewRequest("POST", url, strings.NewReader(data))
	if err != nil {
		log.Printf("[PostRaw] Failed to create request for %s: %v", url, err)
		return nil, fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Accept", requestAccept)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	hash := md5.Sum([]byte(data))
	req.Header.Set("CDC-Checksum", hex.EncodeToString(hash[:]))
	req.Header.Set("Client-ID", state.GetClientID())
	req.Header.Set("Algo-ID", state.GetAlgoID())

	if v := state.GetSchoolID(); v != "" {
		req.Header.Set("CDC-SchoolId", v)
	}
	if v := state.GetDomain(); v != "" {
		req.Header.Set("CDC-Domain", v)
	}
	if v := state.GetArea(); v != "" {
		req.Header.Set("CDC-Area", v)
	}

	resp, err := client.Do(req)
	if err != nil {
		log.Printf("[PostRaw] Request to %s failed: %v", url, err)
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		log.Printf("[PostRaw] %s returned status %d", url, resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Printf("[PostRaw] Failed to read body from %s: %v", url, err)
		return nil, fmt.Errorf("read body: %w", err)
	}
	if len(body) == 0 {
		finalURL := ""
		if resp.Request != nil && resp.Request.URL != nil {
			finalURL = resp.Request.URL.String()
		}
		log.Printf("[PostRaw] Empty response body from %s (status=%d, finalURL=%s)", url, resp.StatusCode, finalURL)
	}

	return body, nil
}

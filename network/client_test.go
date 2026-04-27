package network

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

type testState struct {
	clientID string
	algoID   string
	schoolID string
	domain   string
	area     string
}

func (s *testState) GetClientID() string { return s.clientID }
func (s *testState) GetAlgoID() string   { return s.algoID }
func (s *testState) GetSchoolID() string { return s.schoolID }
func (s *testState) GetDomain() string   { return s.domain }
func (s *testState) GetArea() string     { return s.area }
func (s *testState) SetArea(v string)    { s.area = v }
func (s *testState) SetSchoolID(v string) {
	s.schoolID = v
}
func (s *testState) SetDomain(v string) { s.domain = v }

func TestPostRedirectPreservesBody(t *testing.T) {
	state := &testState{
		clientID: "cid-test",
		algoID:   "00000000-0000-0000-0000-000000000000",
	}

	var server *httptest.Server
	mux := http.NewServeMux()
	mux.HandleFunc("/start", func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		_ = r.Body.Close()
		w.Header().Set("Location", server.URL+"/auth")
		w.WriteHeader(http.StatusFound)
	})

	var gotMethod string
	var gotBody string
	mux.HandleFunc("/auth", func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		body, _ := io.ReadAll(r.Body)
		_ = r.Body.Close()
		gotBody = string(body)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(body)
	})

	server = httptest.NewServer(mux)
	defer server.Close()

	client := NewHTTPClient(state)
	payload := "hello=world&x=1"

	respBody, err := Post(client, server.URL+"/start", payload, state, nil)
	if err != nil {
		t.Fatalf("Post returned error: %v", err)
	}
	if gotMethod != http.MethodPost {
		t.Fatalf("redirected request method = %q, want %q", gotMethod, http.MethodPost)
	}
	if gotBody != payload {
		t.Fatalf("redirected request body = %q, want %q", gotBody, payload)
	}
	if respBody != payload {
		t.Fatalf("Post response body = %q, want %q", respBody, payload)
	}
}

package client

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestParseError(t *testing.T) {
	cases := []struct {
		name, body      string
		msg, tag, reqID string
	}{
		{"flat", `{"error":"bad query","tag":"INVALID_REQUEST","requestId":"r1"}`, "bad query", "INVALID_REQUEST", "r1"},
		{"nested", `{"error":{"message":"nope","code":"NOT_FOUND"}}`, "nope", "NOT_FOUND", ""},
		{"message", `{"message":"Forbidden"}`, "Forbidden", "", ""},
		{"text", `upstream timeout`, "upstream timeout", "", ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			e := ParseError(400, []byte(c.body))
			if e.Message != c.msg || e.Tag != c.tag || e.RequestID != c.reqID {
				t.Fatalf("got %+v", e)
			}
		})
	}
	if got := ParseError(401, []byte(`{}`)).Error(); !strings.Contains(got, "exa config init") {
		t.Fatalf("401 hint missing: %s", got)
	}
}

func TestDoSendsKeyAndRetries(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("x-api-key") != "k" {
			t.Errorf("missing api key header")
		}
		if r.Header.Get("Content-Type") != "application/json" {
			t.Errorf("content type = %q", r.Header.Get("Content-Type"))
		}
		b, _ := io.ReadAll(r.Body)
		if string(b) != `{"query":"q"}` {
			t.Errorf("body = %s", b)
		}
		if calls.Add(1) < 3 {
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		w.Write([]byte(`{"results":[]}`))
	}))
	defer srv.Close()
	c := New("k", srv.URL)
	c.RetryWait = time.Millisecond
	raw, err := c.Post(context.Background(), "/search", map[string]any{"query": "q"})
	if err != nil {
		t.Fatal(err)
	}
	if string(raw) != `{"results":[]}` || calls.Load() != 3 {
		t.Fatalf("raw=%s calls=%d", raw, calls.Load())
	}
}

func TestDoNoRetryOn400(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(`{"error":"numResults too large"}`))
	}))
	defer srv.Close()
	_, err := New("k", srv.URL).Post(context.Background(), "/search", map[string]any{})
	var apiErr *APIError
	if err == nil || !asAPIError(err, &apiErr) || apiErr.Status != 400 || calls.Load() != 1 {
		t.Fatalf("err=%v calls=%d", err, calls.Load())
	}
}

func asAPIError(err error, out **APIError) bool {
	e, ok := err.(*APIError)
	*out = e
	return ok
}

func TestScanSSE(t *testing.T) {
	in := ": keepalive\n\ndata: {\"a\":1}\n\ndata: line1\ndata: line2\n\ndata: [DONE]\n\ndata: after\n\n"
	var got []string
	if err := ScanSSE(strings.NewReader(in), func(d []byte) error {
		got = append(got, string(d))
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	want := []string{`{"a":1}`, "line1\nline2"}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("got %q", got)
	}
}

func TestStream(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Accept") != "text/event-stream" {
			t.Errorf("accept = %q", r.Header.Get("Accept"))
		}
		io.WriteString(w, "data: {\"n\":1}\n\ndata: {\"n\":2}")
	}))
	defer srv.Close()
	var n int
	err := New("k", srv.URL).Stream(context.Background(), Request{Method: "POST", Path: "/answer", Body: json.RawMessage(`{}`)}, func(d []byte) error {
		n++
		return nil
	})
	if err != nil || n != 2 {
		t.Fatalf("err=%v n=%d", err, n)
	}
}

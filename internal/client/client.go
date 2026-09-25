package client

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

// UserAgent is sent on every request; the root command stamps the version in.
var UserAgent = "exa-cli/dev"

// APIError is a non-2xx answer from the Exa API.
type APIError struct {
	Status    int
	Message   string
	Tag       string
	RequestID string
}

func (e *APIError) Error() string {
	msg := e.Message
	if msg == "" {
		msg = http.StatusText(e.Status)
	}
	s := fmt.Sprintf("exa error: HTTP %d: %s", e.Status, msg)
	if e.Tag != "" {
		s += " [" + e.Tag + "]"
	}
	if e.RequestID != "" {
		s += " (request " + e.RequestID + ")"
	}
	if hint := statusHint(e.Status); hint != "" {
		s += "\n" + hint
	}
	return s
}

func statusHint(status int) string {
	switch status {
	case http.StatusUnauthorized:
		return "the API key is missing or invalid; run 'exa config init' or check EXA_API_KEY"
	case http.StatusPaymentRequired:
		return "out of credits; top up at https://dashboard.exa.ai"
	case http.StatusForbidden:
		return "this key is not allowed to use this endpoint or feature"
	case http.StatusTooManyRequests:
		return "rate limited; retry shortly"
	}
	return ""
}

// ParseError builds an APIError from a response body. Exa answers with
// {"error":"...","tag":"...","requestId":"..."}; some surfaces nest the
// message as {"error":{"message":"..."}}, and proxies may send plain text.
func ParseError(status int, body []byte) *APIError {
	e := &APIError{Status: status}
	var head struct {
		Error     json.RawMessage `json:"error"`
		Message   string          `json:"message"`
		Tag       string          `json:"tag"`
		RequestID string          `json:"requestId"`
	}
	if json.Unmarshal(body, &head) != nil {
		e.Message = strings.TrimSpace(string(body))
		if len(e.Message) > 500 {
			e.Message = e.Message[:500] + "…"
		}
		return e
	}
	e.Tag, e.RequestID, e.Message = head.Tag, head.RequestID, head.Message
	if len(head.Error) > 0 {
		var s string
		if json.Unmarshal(head.Error, &s) == nil {
			e.Message = s
		} else {
			var nested struct {
				Message string `json:"message"`
				Code    string `json:"code"`
			}
			if json.Unmarshal(head.Error, &nested) == nil {
				e.Message = nested.Message
				if e.Tag == "" {
					e.Tag = nested.Code
				}
			}
		}
	}
	return e
}

// Client is the Exa HTTP client.
type Client struct {
	apiKey  string
	baseURL string
	http    *http.Client
	Verbose bool
	// RetryWait is the initial backoff for 429/5xx when Retry-After is absent.
	RetryWait time.Duration
	// MaxRetries bounds retries of 429 and 502/503/504.
	MaxRetries int
}

func New(apiKey, baseURL string) *Client {
	return &Client{
		apiKey:  apiKey,
		baseURL: strings.TrimRight(baseURL, "/"),
		// No client-level timeout: deep search, answer and streams can run for
		// minutes. Callers bound requests through the context instead.
		http:       &http.Client{},
		RetryWait:  time.Second,
		MaxRetries: 3,
	}
}

// Request describes one API call.
type Request struct {
	Method  string
	Path    string // e.g. "/search"
	Query   url.Values
	Body    any // marshalled as JSON when non-nil; json.RawMessage passes through
	Headers map[string]string
}

func (c *Client) newHTTPRequest(ctx context.Context, r Request) (*http.Request, []byte, error) {
	var body []byte
	if r.Body != nil {
		switch b := r.Body.(type) {
		case json.RawMessage:
			body = b
		case []byte:
			body = b
		default:
			var err error
			body, err = json.Marshal(b)
			if err != nil {
				return nil, nil, fmt.Errorf("encode request: %w", err)
			}
		}
	}
	u := c.baseURL + r.Path
	if len(r.Query) > 0 {
		u += "?" + r.Query.Encode()
	}
	var rd io.Reader
	if body != nil {
		rd = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(ctx, r.Method, u, rd)
	if err != nil {
		return nil, nil, err
	}
	req.Header.Set("x-api-key", c.apiKey)
	req.Header.Set("User-Agent", UserAgent)
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	for k, v := range r.Headers {
		req.Header.Set(k, v)
	}
	return req, body, nil
}

func retryable(status int) bool {
	switch status {
	case http.StatusTooManyRequests, http.StatusBadGateway, http.StatusServiceUnavailable, http.StatusGatewayTimeout:
		return true
	}
	return false
}

// do sends the request with retries and returns the final response. The caller
// owns resp.Body on success; on a non-2xx answer the body is consumed and an
// *APIError is returned.
func (c *Client) do(ctx context.Context, r Request) (*http.Response, error) {
	wait := c.RetryWait
	if wait <= 0 {
		wait = time.Second
	}
	for attempt := 0; ; attempt++ {
		req, body, err := c.newHTTPRequest(ctx, r)
		if err != nil {
			return nil, err
		}
		if c.Verbose {
			fmt.Fprintf(os.Stderr, "→ %s %s\n", req.Method, req.URL.String())
			if body != nil {
				fmt.Fprintf(os.Stderr, "%s\n", body)
			}
		}
		resp, err := c.http.Do(req)
		if err != nil {
			return nil, err
		}
		if resp.StatusCode >= 200 && resp.StatusCode < 300 {
			if c.Verbose {
				fmt.Fprintf(os.Stderr, "← HTTP %d\n", resp.StatusCode)
			}
			return resp, nil
		}
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if c.Verbose {
			fmt.Fprintf(os.Stderr, "← HTTP %d\n%s\n", resp.StatusCode, b)
		}
		if !retryable(resp.StatusCode) || attempt >= c.MaxRetries {
			return nil, ParseError(resp.StatusCode, b)
		}
		d := wait
		if ra := resp.Header.Get("Retry-After"); ra != "" {
			if secs, e := strconv.Atoi(ra); e == nil && secs > 0 {
				d = time.Duration(secs) * time.Second
			}
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(d):
		}
		wait *= 2
	}
}

// Do sends a request and returns the raw JSON body.
func (c *Client) Do(ctx context.Context, r Request) (json.RawMessage, error) {
	resp, err := c.do(ctx, r)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read Exa response: %w", err)
	}
	if c.Verbose {
		fmt.Fprintf(os.Stderr, "%s\n", b)
	}
	return b, nil
}

// Post is Do for a JSON POST.
func (c *Client) Post(ctx context.Context, path string, body any) (json.RawMessage, error) {
	return c.Do(ctx, Request{Method: http.MethodPost, Path: path, Body: body})
}

// Stream sends a request and calls fn with the payload of every server-sent
// "data:" line until the stream ends, "[DONE]" arrives, or fn returns an error.
func (c *Client) Stream(ctx context.Context, r Request, fn func(data []byte) error) error {
	if r.Headers == nil {
		r.Headers = map[string]string{}
	}
	r.Headers["Accept"] = "text/event-stream"
	resp, err := c.do(ctx, r)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return ScanSSE(resp.Body, func(data []byte) error {
		if c.Verbose {
			fmt.Fprintf(os.Stderr, "← %s\n", data)
		}
		return fn(data)
	})
}

// ScanSSE reads server-sent events and passes each event's data to fn.
// Multi-line data fields are joined with "\n" per the SSE spec.
func ScanSSE(r io.Reader, fn func(data []byte) error) error {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64*1024), 16*1024*1024)
	var buf []byte
	flush := func() error {
		if buf == nil {
			return nil
		}
		data := buf
		buf = nil
		if string(data) == "[DONE]" {
			return io.EOF
		}
		return fn(data)
	}
	for sc.Scan() {
		line := sc.Text()
		if line == "" {
			if err := flush(); err != nil {
				if err == io.EOF {
					return nil
				}
				return err
			}
			continue
		}
		if strings.HasPrefix(line, ":") {
			continue
		}
		if v, ok := strings.CutPrefix(line, "data:"); ok {
			v = strings.TrimPrefix(v, " ")
			if buf != nil {
				buf = append(buf, '\n')
			} else {
				buf = []byte{}
			}
			buf = append(buf, v...)
		}
	}
	if err := sc.Err(); err != nil {
		return err
	}
	if err := flush(); err != nil && err != io.EOF {
		return err
	}
	return nil
}

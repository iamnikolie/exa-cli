package cmd

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// resetFlags restores every flag to its default: cobra keeps parsed values in
// package variables between Execute calls.
func resetFlags(c *cobra.Command) {
	reset := func(f *pflag.Flag) {
		if sv, ok := f.Value.(pflag.SliceValue); ok {
			sv.Replace(nil)
		} else {
			f.Value.Set(f.DefValue)
		}
		f.Changed = false
	}
	c.Flags().VisitAll(reset)
	c.PersistentFlags().VisitAll(reset)
	for _, sub := range c.Commands() {
		resetFlags(sub)
	}
}

type call struct {
	method, path, query string
	headers             http.Header
	body                map[string]any
}

// run executes the CLI against a fake Exa server answering with reply.
func run(t *testing.T, reply string, args ...string) (string, string, []call, error) {
	t.Helper()
	var calls []call
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		c := call{method: r.Method, path: r.URL.Path, query: r.URL.RawQuery, headers: r.Header}
		if len(b) > 0 {
			if err := json.Unmarshal(b, &c.body); err != nil {
				t.Errorf("request body is not JSON: %s", b)
			}
		}
		calls = append(calls, c)
		io.WriteString(w, reply)
	}))
	defer srv.Close()
	t.Setenv("EXA_API_KEY", "test-key")
	t.Setenv("EXA_BASE_URL", srv.URL)
	t.Setenv("EXA_CLI_HOME", t.TempDir())

	resetFlags(rootCmd)
	var out, errb bytes.Buffer
	stdout, stderr = &out, &errb
	defer func() { stdout, stderr = os.Stdout, os.Stderr }()
	rootCmd.SetArgs(args)
	rootCmd.SetOut(&out)
	rootCmd.SetErr(&errb)
	err := rootCmd.Execute()
	return out.String(), errb.String(), calls, err
}

const searchReply = `{"requestId":"r","results":[
 {"title":"Go 1.27 released","url":"https://go.dev/blog/go1.27","publishedDate":"2026-08-12T00:00:00.000Z","author":"The Go Team","highlights":["Go 1.27 adds X."],"summary":"Release post."},
 {"title":"Other","url":"https://example.com/o"}],
 "costDollars":{"total":0.007}}`

func TestSearchDefaultsAndMarkdown(t *testing.T) {
	out, errOut, calls, err := run(t, searchReply, "search", "go", "release", "--after", "2026-01-01", "-n", "2", "--include-domain", "go.dev,golang.org")
	if err != nil {
		t.Fatal(err)
	}
	if len(calls) != 1 || calls[0].path != "/search" || calls[0].headers.Get("x-api-key") != "test-key" {
		t.Fatalf("calls: %+v", calls)
	}
	b := calls[0].body
	if b["query"] != "go release" || b["numResults"] != float64(2) || b["startPublishedDate"] != "2026-01-01T00:00:00.000Z" {
		t.Fatalf("body: %v", b)
	}
	if d := b["includeDomains"].([]any); len(d) != 2 {
		t.Fatalf("domains: %v", d)
	}
	if c := b["contents"].(map[string]any); c["highlights"] != true || c["text"] != nil {
		t.Fatalf("default contents should be highlights only: %v", c)
	}
	for _, want := range []string{"### 1. Go 1.27 released", "https://go.dev/blog/go1.27 · 2026-08-12 · The Go Team", "> Go 1.27 adds X.", "**Summary:** Release post.", "### 2. Other"} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in:\n%s", want, out)
		}
	}
	if !strings.Contains(errOut, "(cost $0.007)") {
		t.Errorf("cost footer missing: %q", errOut)
	}
}

func TestSearchNoContentsTableAndFields(t *testing.T) {
	out, _, calls, err := run(t, searchReply, "search", "q", "--no-contents", "--format", "table")
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := calls[0].body["contents"]; ok {
		t.Fatalf("--no-contents must omit contents: %v", calls[0].body)
	}
	if !strings.HasPrefix(out, "| title | url | publishedDate |\n") {
		t.Fatalf("table:\n%s", out)
	}

	out, _, _, err = run(t, searchReply, "search", "q", "--json", "--fields", "url")
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(out) != `[{"url":"https://go.dev/blog/go1.27"},{"url":"https://example.com/o"}]` {
		t.Fatalf("json fields: %s", out)
	}
}

func TestSearchDeepOutput(t *testing.T) {
	reply := `{"results":[{"title":"S","url":"https://s"}],"output":{"content":{"ceo":"X"},"grounding":[]}}`
	out, _, calls, err := run(t, reply, "search", "who", "--type", "deep", "--schema", `{"type":"object"}`, "--also", "a", "--also", "b")
	if err != nil {
		t.Fatal(err)
	}
	b := calls[0].body
	if b["type"] != "deep" || b["outputSchema"].(map[string]any)["type"] != "object" || len(b["additionalQueries"].([]any)) != 2 {
		t.Fatalf("body: %v", b)
	}
	if !strings.Contains(out, "## Output") || !strings.Contains(out, `"ceo": "X"`) || !strings.Contains(out, "## Sources") {
		t.Fatalf("out:\n%s", out)
	}
}

func TestFetchDefaultsToTextAndReportsStatuses(t *testing.T) {
	reply := `{"results":[{"url":"https://a","title":"A","text":"Body A"}],"statuses":[{"id":"https://a","status":"success"},{"id":"https://b","status":"error","error":{"tag":"CRAWL_NOT_FOUND","httpStatusCode":404}}]}`
	out, errOut, calls, err := run(t, reply, "fetch", "https://a", "https://b", "--max-age", "0", "--links", "5")
	if err != nil {
		t.Fatal(err)
	}
	b := calls[0].body
	if calls[0].path != "/contents" || b["text"] != true || b["maxAgeHours"] != float64(0) || len(b["urls"].([]any)) != 2 {
		t.Fatalf("body: %v", b)
	}
	if b["extras"].(map[string]any)["links"] != float64(5) {
		t.Fatalf("extras: %v", b["extras"])
	}
	if !strings.Contains(out, "Body A") {
		t.Fatalf("out:\n%s", out)
	}
	if !strings.Contains(errOut, "! https://b: error CRAWL_NOT_FOUND (HTTP 404)") {
		t.Fatalf("stderr: %q", errOut)
	}
}

func TestSimilarAndCode(t *testing.T) {
	_, _, calls, err := run(t, searchReply, "similar", "https://x.com/a", "--exclude-source-domain", "--summary-query", "what?")
	if err != nil {
		t.Fatal(err)
	}
	b := calls[0].body
	if calls[0].path != "/findSimilar" || b["url"] != "https://x.com/a" || b["excludeSourceDomain"] != true {
		t.Fatalf("body: %v", b)
	}
	if s := b["contents"].(map[string]any)["summary"].(map[string]any); s["query"] != "what?" {
		t.Fatalf("summary: %v", s)
	}

	_, _, calls, err = run(t, searchReply, "code", "go", "retry")
	if err != nil {
		t.Fatal(err)
	}
	b = calls[0].body
	if b["type"] != "fast" || b["contents"].(map[string]any)["highlights"].(map[string]any)["query"] != "go retry" {
		t.Fatalf("code body: %v", b)
	}
}

func TestAnswer(t *testing.T) {
	reply := `{"answer":"Paris.","citations":[{"title":"France","url":"https://fr"}],"costDollars":{"total":0.005}}`
	out, _, calls, err := run(t, reply, "answer", "capital", "of", "France?", "--model", "exa-pro", "--country", "fr")
	if err != nil {
		t.Fatal(err)
	}
	b := calls[0].body
	if b["query"] != "capital of France?" || b["model"] != "exa-pro" || b["userLocation"] != "FR" {
		t.Fatalf("body: %v", b)
	}
	if out != "Paris.\n\nSources:\n[1] France — https://fr\n" {
		t.Fatalf("out: %q", out)
	}
}

func TestAnswerStream(t *testing.T) {
	reply := "data: {\"choices\":[{\"delta\":{\"content\":\"Par\"}}]}\n\n" +
		"data: {\"choices\":[{\"delta\":{\"content\":\"is.\"}}]}\n\n" +
		"data: {\"citations\":[{\"title\":\"F\",\"url\":\"https://fr\"}]}\n\n"
	out, _, calls, err := run(t, reply, "answer", "q", "--stream")
	if err != nil {
		t.Fatal(err)
	}
	if calls[0].body["stream"] != true {
		t.Fatalf("stream flag not sent: %v", calls[0].body)
	}
	if out != "Paris.\n\nSources:\n[1] F — https://fr\n" {
		t.Fatalf("out: %q", out)
	}
}

func TestAgentRunSendsBetaAndBudget(t *testing.T) {
	out, _, calls, err := run(t, `{"id":"run_1","status":"queued"}`, "agent", "run", "find", "things", "--effort", "max", "--max-cost", "2.5", "--data-source", "crunchbase")
	if err != nil {
		t.Fatal(err)
	}
	c := calls[0]
	if c.path != "/agent/runs" || c.headers.Get("Exa-Beta") != agentBeta+","+agentMaxEffortBeta {
		t.Fatalf("call: %s %v", c.path, c.headers)
	}
	if c.body["effort"] != "max" || c.body["budget"].(map[string]any)["maxCostDollars"] != 2.5 {
		t.Fatalf("body: %v", c.body)
	}
	if ds := c.body["dataSources"].([]any); ds[0].(map[string]any)["provider"] != "crunchbase" {
		t.Fatalf("dataSources: %v", ds)
	}
	if out != "run_1\n" {
		t.Fatalf("out: %q", out)
	}
}

func TestAgentGetRendersOutput(t *testing.T) {
	reply := `{"id":"run_1","status":"completed","stopReason":"schema_satisfied","output":{"text":"Done.","structured":{"n":1},"grounding":[{"field":"n","confidence":"high","citations":[{"url":"https://c"}]}]}}`
	out, _, calls, err := run(t, reply, "agent", "get", "run_1")
	if err != nil {
		t.Fatal(err)
	}
	if calls[0].headers.Get("Exa-Beta") != agentBeta {
		t.Fatalf("beta header missing on get")
	}
	for _, want := range []string{"run_1 · completed (schema_satisfied)", "Done.", `"n": 1`, "- n [high]: https://c"} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in:\n%s", want, out)
		}
	}
}

func TestListShowsCursor(t *testing.T) {
	reply := `{"data":[{"id":"run_1","status":"completed","stopReason":"schema_satisfied","createdAt":"2026-09-25T09:36:19.549Z","request":{"query":"x"}}],"hasMore":true,"nextCursor":"c2"}`
	out, errOut, calls, err := run(t, reply, "agent", "list", "--limit", "1")
	if err != nil {
		t.Fatal(err)
	}
	if calls[0].query != "limit=1" || calls[0].headers.Get("Exa-Beta") != agentBeta {
		t.Fatalf("call: %+v", calls[0])
	}
	if !strings.Contains(out, "| run_1 | completed | schema_satisfied | 2026-09-25T09:36:19.549Z | x |") || !strings.Contains(errOut, "--cursor c2") {
		t.Fatalf("out:\n%s\nerr: %s", out, errOut)
	}
}

func TestFetchAllFailedExitsNonZero(t *testing.T) {
	reply := `{"results":[],"statuses":[{"id":"https://b","status":"error","error":{"tag":"CRAWL_NOT_FOUND"}}]}`
	_, errOut, _, err := run(t, reply, "fetch", "https://b")
	if err == nil || !strings.Contains(errOut, "! https://b") {
		t.Fatalf("err=%v stderr=%q", err, errOut)
	}
}

func TestSourcesDedupeURLTitles(t *testing.T) {
	reply := `{"answer":"A","citations":[{"title":"https://u","url":"https://u"},{"title":"","url":"https://v"}]}`
	out, _, _, err := run(t, reply, "answer", "q")
	if err != nil {
		t.Fatal(err)
	}
	if out != "A\n\nSources:\n[1] https://u\n[2] https://v\n" {
		t.Fatalf("out: %q", out)
	}
}

func TestAPIPassthrough(t *testing.T) {
	out, _, calls, err := run(t, `{"data":[]}`, "api", "get", "v0/websets", "-q", "limit=5", "-H", "Exa-Beta: x")
	if err != nil {
		t.Fatal(err)
	}
	c := calls[0]
	if c.method != "GET" || c.path != "/v0/websets" || c.query != "limit=5" || c.headers.Get("Exa-Beta") != "x" {
		t.Fatalf("call: %+v", c)
	}
	if strings.TrimSpace(out) != `{"data":[]}` {
		t.Fatalf("out: %s", out)
	}
}

func TestConfigInitAndShow(t *testing.T) {
	home := t.TempDir()
	t.Setenv("EXA_CLI_HOME", home)
	t.Setenv("EXA_API_KEY", "")
	resetFlags(rootCmd)
	var out bytes.Buffer
	stdout, stderr = &out, io.Discard
	stdin = strings.NewReader("abcd-1234-efgh-5678\n")
	defer func() { stdout, stderr, stdin = os.Stdout, os.Stderr, os.Stdin }()

	rootCmd.SetArgs([]string{"--config", "work", "config", "init"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(home, "work", "config.yaml")
	info, err := os.Stat(path)
	if err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("config file: %v %v", info, err)
	}
	out.Reset()
	resetFlags(rootCmd)
	rootCmd.SetArgs([]string{"--config", "work", "config", "show", "--json"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), `"api_key":"set (…5678)"`) || strings.Contains(out.String(), "abcd") {
		t.Fatalf("show leaks or misses key state: %s", out.String())
	}
}

func TestMissingKey(t *testing.T) {
	t.Setenv("EXA_API_KEY", "")
	t.Setenv("EXA_CLI_HOME", t.TempDir())
	resetFlags(rootCmd)
	stderr = io.Discard
	defer func() { stderr = os.Stderr }()
	rootCmd.SetOut(io.Discard)
	rootCmd.SetErr(io.Discard)
	rootCmd.SetArgs([]string{"search", "x"})
	if err := rootCmd.Execute(); err == nil || !strings.Contains(err.Error(), "exa config init") {
		t.Fatalf("err: %v", err)
	}
}

func TestIsoDate(t *testing.T) {
	if isoDate("2026-02-03", true) != "2026-02-03T23:59:59.999Z" || isoDate("2026-02-03T10:00:00Z", false) != "2026-02-03T10:00:00Z" {
		t.Fatal("isoDate")
	}
}

func TestContentConflicts(t *testing.T) {
	c := contentFlags{noContents: true, text: true, maxAge: -1}
	if _, err := c.build("highlights"); err == nil {
		t.Fatal("expected conflict error")
	}
	f := filterFlags{includeDomains: []string{"a"}, excludeDomains: []string{"b"}}
	if err := f.apply(map[string]any{}); err == nil {
		t.Fatal("expected domain conflict error")
	}
}

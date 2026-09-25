package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/iamnikolie/exa-cli/internal/client"
	"github.com/spf13/cobra"
)

// Agent runs (/agent/runs) are long-running tasks: run returns an id, get
// polls it, list pages through them.

type taskKind struct {
	name     string   // noun for progress lines
	base     string   // API path
	idField  string   // id key in responses
	terminal []string // statuses that end a wait
	headers  func() map[string]string
}

// Agent runs are a beta surface: every call carries the Exa-Beta header.
const (
	agentBeta          = "agent-2026-05-07"
	agentMaxEffortBeta = "agent-max-effort-2026-07-27"
)

var agentKind = taskKind{
	name:     "agent run",
	base:     "/agent/runs",
	idField:  "id",
	terminal: []string{"completed", "failed", "cancelled", "canceled"},
	headers:  func() map[string]string { return map[string]string{"Exa-Beta": agentBeta} },
}

func (k taskKind) hdrs() map[string]string {
	if k.headers == nil {
		return nil
	}
	return k.headers()
}

func (k taskKind) isTerminal(status string) bool {
	for _, t := range k.terminal {
		if status == t {
			return true
		}
	}
	return false
}

func (k taskKind) get(cmd *cobra.Command, id string, q url.Values) (map[string]any, error) {
	ctx, cancel := reqCtx(cmd)
	defer cancel()
	raw, err := cli.Do(ctx, client.Request{Method: http.MethodGet, Path: k.base + "/" + url.PathEscape(id), Query: q, Headers: k.hdrs()})
	if err != nil {
		return nil, err
	}
	return decodeObject(raw)
}

// wait polls until the task reaches a terminal status or maxWait elapses.
func (k taskKind) wait(cmd *cobra.Command, id string, every, maxWait time.Duration, q url.Values) (map[string]any, error) {
	deadline := time.Now().Add(maxWait)
	last := ""
	for {
		v, err := k.get(cmd, id, q)
		if err != nil {
			return nil, err
		}
		status := str(v, "status")
		if status != last {
			fmt.Fprintf(stderr, "%s %s: %s\n", k.name, id, status)
			last = status
		}
		if k.isTerminal(status) {
			return v, nil
		}
		if maxWait > 0 && time.Now().After(deadline) {
			return nil, fmt.Errorf("%s %s still %s after %s; resume with the same id", k.name, id, status, maxWait)
		}
		select {
		case <-cmd.Context().Done():
			return nil, cmd.Context().Err()
		case <-time.After(every):
		}
	}
}

func (k taskKind) list(cmd *cobra.Command, limit int, cursor string, cols []string, tidy func(map[string]any) map[string]any) error {
	q := url.Values{}
	if limit > 0 {
		q.Set("limit", strconv.Itoa(limit))
	}
	if cursor != "" {
		q.Set("cursor", cursor)
	}
	ctx, cancel := reqCtx(cmd)
	defer cancel()
	raw, err := cli.Do(ctx, client.Request{Method: http.MethodGet, Path: k.base, Query: q, Headers: k.hdrs()})
	if err != nil {
		return err
	}
	v, err := decodeObject(raw)
	if err != nil {
		return err
	}
	if format() == "json" && len(fieldsFlag) == 0 {
		return writeJSON(stdout, v)
	}
	rows := objects(v["data"])
	if format() != "json" {
		for i, r := range rows {
			rows[i] = tidy(r)
		}
	}
	if err := emitRows(rows, cols); err != nil {
		return err
	}
	if next := str(v, "nextCursor"); next != "" && str(v, "hasMore") != "false" {
		fmt.Fprintf(stderr, "(more: --cursor %s)\n", next)
	}
	return nil
}

var (
	pollEvery       time.Duration
	waitMax         time.Duration
	listLimit       int
	listCursor      string
	agentEffort     string
	agentSystem     string
	agentSchema     string
	agentMaxCost    float64
	agentMaxSecs    int
	agentPrevious   string
	agentInput      string
	agentWait       bool
	agentSources    []string
	agentMetadata   string
	agentEventLimit int
)

// ---- agent ----

var agentCmd = &cobra.Command{
	Use:   "agent",
	Short: "Exa Agent runs (beta): autonomous search agents with structured, grounded output",
}

func buildAgentBody(query string) (map[string]any, []string, error) {
	body := map[string]any{"query": query}
	betas := []string{agentBeta}
	if agentSystem != "" {
		body["systemPrompt"] = agentSystem
	}
	if agentEffort != "" {
		body["effort"] = agentEffort
		if agentEffort == "max" {
			betas = append(betas, agentMaxEffortBeta)
		}
	}
	if agentSchema != "" {
		schema, err := readJSONArg("schema", agentSchema)
		if err != nil {
			return nil, nil, err
		}
		body["outputSchema"] = schema
	}
	budget := map[string]any{}
	if agentMaxCost > 0 {
		budget["maxCostDollars"] = agentMaxCost
	}
	if agentMaxSecs > 0 {
		budget["maxDurationSeconds"] = agentMaxSecs
	}
	if len(budget) > 0 {
		body["budget"] = budget
	}
	if agentPrevious != "" {
		body["previousRunId"] = agentPrevious
	}
	if agentInput != "" {
		in, err := readJSONArg("input", agentInput)
		if err != nil {
			return nil, nil, err
		}
		body["input"] = in
	}
	if len(agentSources) > 0 {
		ds := make([]map[string]any, len(agentSources))
		for i, p := range agentSources {
			ds[i] = map[string]any{"provider": p}
		}
		body["dataSources"] = ds
	}
	if agentMetadata != "" {
		md, err := readJSONArg("metadata", agentMetadata)
		if err != nil {
			return nil, nil, err
		}
		body["metadata"] = md
	}
	return body, betas, nil
}

var agentRunCmd = &cobra.Command{
	Use:   "run <query>",
	Short: "Start an agent run (prints the id; --wait blocks for the output)",
	Example: `  exa agent run "Find 10 seed-stage devtools startups in Berlin with founders' LinkedIn" --wait
  exa agent run "Pricing of the top 5 transactional email APIs" --schema @schema.json --effort high --wait
  exa agent run "Now add their HQ city" --previous <run-id> --wait`,
	Args: cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		query, err := readArg(strings.Join(args, " "))
		if err != nil {
			return err
		}
		body, betas, err := buildAgentBody(query)
		if err != nil {
			return err
		}
		ctx, cancel := reqCtx(cmd)
		defer cancel()
		raw, err := cli.Do(ctx, client.Request{
			Method:  http.MethodPost,
			Path:    agentKind.base,
			Body:    body,
			Headers: map[string]string{"Exa-Beta": strings.Join(betas, ",")},
		})
		if err != nil {
			return err
		}
		v, err := decodeObject(raw)
		if err != nil {
			return err
		}
		id := str(v, "id")
		if !agentWait {
			if format() == "json" {
				return writeJSON(stdout, v)
			}
			fmt.Fprintln(stdout, id)
			fmt.Fprintf(stderr, "agent run started (%s); follow with: exa agent get %s --wait\n", str(v, "status"), id)
			return nil
		}
		done, err := agentKind.wait(cmd, id, pollEvery, waitMax, nil)
		if err != nil {
			return err
		}
		return printAgentRun(done)
	},
}

var agentGetCmd = &cobra.Command{
	Use:   "get <id>",
	Short: "Show an agent run's status and output",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		var v map[string]any
		var err error
		if agentWait {
			v, err = agentKind.wait(cmd, args[0], pollEvery, waitMax, nil)
		} else {
			v, err = agentKind.get(cmd, args[0], nil)
		}
		if err != nil {
			return err
		}
		return printAgentRun(v)
	},
}

var agentListCmd = &cobra.Command{
	Use:   "list",
	Short: "List agent runs, newest first",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		return agentKind.list(cmd, listLimit, listCursor,
			[]string{"id", "status", "stopReason", "createdAt", "request.query"},
			func(r map[string]any) map[string]any {
				if req, ok := r["request"].(map[string]any); ok {
					req["query"] = truncate(str(req, "query"), 80)
				}
				return r
			})
	},
}

var agentCancelCmd = &cobra.Command{
	Use:   "cancel <id>",
	Short: "Cancel a queued or running agent run",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx, cancel := reqCtx(cmd)
		defer cancel()
		raw, err := cli.Do(ctx, client.Request{Method: http.MethodPost, Path: agentKind.base + "/" + url.PathEscape(args[0]) + "/cancel", Headers: agentKind.hdrs()})
		if err != nil {
			return err
		}
		if format() == "json" {
			return writeRaw(raw)
		}
		v, err := decodeObject(raw)
		if err != nil {
			return err
		}
		fmt.Fprintf(stdout, "%s %s\n", args[0], str(v, "status"))
		return nil
	},
}

var agentEventsCmd = &cobra.Command{
	Use:   "events <id>",
	Short: "List stored events of an agent run",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		k := agentKind
		k.base = agentKind.base + "/" + url.PathEscape(args[0]) + "/events"
		return k.list(cmd, agentEventLimit, listCursor, []string{"createdAt", "event", "data"},
			func(r map[string]any) map[string]any {
				if d, ok := r["data"]; ok {
					b, _ := json.Marshal(d)
					r["data"] = truncate(string(b), 160)
				}
				return r
			})
	},
}

func printAgentRun(v map[string]any) error {
	if format() == "json" {
		if len(fieldsFlag) > 0 {
			return writeJSON(stdout, project(v, fieldsFlag))
		}
		return writeJSON(stdout, v)
	}
	writeAgentRun(stdout, v)
	costFooter(v)
	return nil
}

func writeAgentRun(w io.Writer, v map[string]any) {
	line := fmt.Sprintf("**agent run:** %s · %s", str(v, "id"), str(v, "status"))
	if r := str(v, "stopReason"); r != "" {
		line += " (" + r + ")"
	}
	fmt.Fprintln(w, line)
	if msg := str(v, "error.message"); msg != "" {
		fmt.Fprintf(w, "**error:** %s\n", msg)
	}
	out, ok := v["output"].(map[string]any)
	if !ok {
		return
	}
	if t := strings.TrimSpace(str(out, "text")); t != "" {
		fmt.Fprintf(w, "\n%s\n", t)
	}
	if s, ok := out["structured"]; ok && s != nil {
		fmt.Fprintln(w, "\n```json")
		fmt.Fprintln(w, prettyJSON(s))
		fmt.Fprintln(w, "```")
	}
	if g := objects(out["grounding"]); len(g) > 0 {
		fmt.Fprintln(w, "\nGrounding:")
		for _, e := range g {
			var urls []string
			for _, c := range objects(e["citations"]) {
				urls = append(urls, str(c, "url"))
			}
			conf := str(e, "confidence")
			if conf != "" {
				conf = " [" + conf + "]"
			}
			fmt.Fprintf(w, "- %s%s: %s\n", str(e, "field"), conf, strings.Join(urls, ", "))
		}
	}
}

func init() {
	for _, c := range []*cobra.Command{agentRunCmd, agentGetCmd} {
		c.Flags().DurationVar(&pollEvery, "poll", 5*time.Second, "polling interval while waiting")
		c.Flags().DurationVar(&waitMax, "wait-timeout", 30*time.Minute, "give up waiting after this long (0 = never); the task keeps running")
	}
	for _, c := range []*cobra.Command{agentListCmd} {
		c.Flags().IntVar(&listLimit, "limit", 20, "page size")
		c.Flags().StringVar(&listCursor, "cursor", "", "cursor from a previous page")
	}
	agentEventsCmd.Flags().IntVar(&agentEventLimit, "limit", 50, "page size")
	agentEventsCmd.Flags().StringVar(&listCursor, "cursor", "", "cursor from a previous page")

	fl := agentRunCmd.Flags()
	fl.StringVar(&agentEffort, "effort", "", "minimal, low, medium, high, xhigh, auto, ultra or max")
	fl.StringVar(&agentSystem, "system-prompt", "", "instructions that steer the agent")
	fl.StringVar(&agentSchema, "schema", "", "JSON schema for structured output: inline, @file or -")
	fl.Float64Var(&agentMaxCost, "max-cost", 0, "spend ceiling in USD (auto and ultra efforts)")
	fl.IntVar(&agentMaxSecs, "max-duration", 0, "soft wall-clock ceiling in seconds (ultra effort)")
	fl.StringVar(&agentPrevious, "previous", "", "continue from a previous run id")
	fl.StringVar(&agentInput, "input", "", `input rows as JSON {"data":[...],"exclusion":[...]}: inline, @file or -`)
	fl.StringSliceVar(&agentSources, "data-source", nil, "Exa Connect provider to enable (repeat)")
	fl.StringVar(&agentMetadata, "metadata", "", "JSON object stored with the run")
	fl.BoolVar(&agentWait, "wait", false, "block until the run finishes and print the output")
	agentGetCmd.Flags().BoolVar(&agentWait, "wait", false, "poll until the run finishes")
	agentCmd.AddCommand(agentRunCmd, agentGetCmd, agentListCmd, agentCancelCmd, agentEventsCmd)

	rootCmd.AddCommand(agentCmd)
}

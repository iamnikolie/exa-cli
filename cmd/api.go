package cmd

import (
	"fmt"
	"net/url"
	"strings"

	"github.com/iamnikolie/exa-cli/internal/client"
	"github.com/spf13/cobra"
)

var (
	apiData    string
	apiQuery   []string
	apiHeaders []string
)

// apiCmd is the escape hatch for endpoints without a dedicated command
// (Websets, monitors, webhooks, imports, new betas).
var apiCmd = &cobra.Command{
	Use:   "api <METHOD> <path>",
	Short: "Call any Exa API endpoint and print the JSON response",
	Example: `  exa api GET /websets/v0/websets -q limit=5
  exa api POST /websets/v0/websets -d '{"search":{"query":"AI labs in Europe","count":10}}'
  exa api GET /agent/runs -H Exa-Beta:agent-2026-05-07
  exa api POST /search -d @body.json`,
	Args: cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		req, err := buildAPIRequest(args[0], args[1], apiData, apiQuery, apiHeaders)
		if err != nil {
			return err
		}
		ctx, cancel := reqCtx(cmd)
		defer cancel()
		raw, err := cli.Do(ctx, req)
		if err != nil {
			return err
		}
		if len(strings.TrimSpace(string(raw))) == 0 {
			return nil
		}
		return writeRaw(raw)
	},
}

func buildAPIRequest(method, path, data string, query, headers []string) (client.Request, error) {
	req := client.Request{Method: strings.ToUpper(method), Path: path}
	if !strings.HasPrefix(req.Path, "/") {
		req.Path = "/" + req.Path
	}
	if data != "" {
		body, err := readJSONArg("data", data)
		if err != nil {
			return req, err
		}
		req.Body = body
	}
	if len(query) > 0 {
		req.Query = url.Values{}
		for _, kv := range query {
			k, v, ok := strings.Cut(kv, "=")
			if !ok {
				return req, fmt.Errorf("-q %q: want key=value", kv)
			}
			req.Query.Add(k, v)
		}
	}
	if len(headers) > 0 {
		req.Headers = map[string]string{}
		for _, kv := range headers {
			k, v, ok := strings.Cut(kv, ":")
			if !ok {
				return req, fmt.Errorf("-H %q: want Name:value", kv)
			}
			req.Headers[strings.TrimSpace(k)] = strings.TrimSpace(v)
		}
	}
	return req, nil
}

func init() {
	apiCmd.Flags().StringVarP(&apiData, "data", "d", "", "JSON request body: inline, @file or -")
	apiCmd.Flags().StringArrayVarP(&apiQuery, "query", "q", nil, "query parameter key=value (repeat)")
	apiCmd.Flags().StringArrayVarP(&apiHeaders, "header", "H", nil, "extra header Name:value (repeat)")
	rootCmd.AddCommand(apiCmd)
}

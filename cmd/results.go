package cmd

import (
	"fmt"
	"io"
	"strings"

	"github.com/spf13/cobra"
)

var resultCols = []string{"title", "url", "publishedDate"}

// printResults renders a /search, /findSimilar or /contents response.
func printResults(raw []byte) error {
	v, err := decodeObject(raw)
	if err != nil {
		return err
	}
	reportStatuses(v)
	results := objects(v["results"])
	switch format() {
	case "json":
		if len(fieldsFlag) == 0 {
			return writeJSON(stdout, v)
		}
		return emitRows(results, nil)
	case "table", "csv", "tsv":
		return emitRows(results, resultCols)
	}
	if len(fieldsFlag) > 0 {
		return emitRows(results, nil)
	}
	writeResultsMarkdown(stdout, v, results)
	costFooter(v)
	return nil
}

// reportStatuses surfaces per-URL fetch failures from /contents on stderr.
func reportStatuses(v map[string]any) {
	for _, s := range objects(v["statuses"]) {
		if str(s, "status") == "success" {
			continue
		}
		msg := str(s, "error.tag")
		if code := str(s, "error.httpStatusCode"); code != "" {
			msg += " (HTTP " + code + ")"
		}
		fmt.Fprintf(stderr, "! %s: %s %s\n", str(s, "id"), str(s, "status"), msg)
	}
}

func writeResultsMarkdown(w io.Writer, v map[string]any, results []map[string]any) {
	if out, ok := v["output"]; ok && out != nil {
		fmt.Fprintln(w, "## Output")
		fmt.Fprintln(w)
		content, _ := getPath(out, "content")
		if s, ok := content.(string); ok {
			fmt.Fprintln(w, strings.TrimSpace(s))
		} else if content != nil {
			fmt.Fprintln(w, "```json")
			fmt.Fprintln(w, prettyJSON(content))
			fmt.Fprintln(w, "```")
		}
		fmt.Fprintln(w)
		if len(results) > 0 {
			fmt.Fprintln(w, "## Sources")
			fmt.Fprintln(w)
		}
	}
	if len(results) == 0 {
		fmt.Fprintln(w, "_No results._")
		return
	}
	for i, r := range results {
		writeResult(w, i+1, r)
	}
}

func writeResult(w io.Writer, n int, r map[string]any) {
	title := str(r, "title")
	if title == "" {
		title = str(r, "url")
	}
	fmt.Fprintf(w, "### %d. %s\n", n, oneLine(title))
	meta := []string{str(r, "url")}
	if d := str(r, "publishedDate"); d != "" {
		if len(d) >= 10 {
			d = d[:10]
		}
		meta = append(meta, d)
	}
	if a := str(r, "author"); a != "" {
		meta = append(meta, oneLine(a))
	}
	fmt.Fprintln(w, strings.Join(meta, " · "))

	if s := str(r, "summary"); s != "" {
		fmt.Fprintf(w, "\n**Summary:** %s\n", strings.TrimSpace(s))
	}
	if hl, ok := r["highlights"].([]any); ok && len(hl) > 0 {
		fmt.Fprintln(w)
		for _, h := range hl {
			if s, ok := h.(string); ok && strings.TrimSpace(s) != "" {
				fmt.Fprintf(w, "> %s\n", oneLine(strings.TrimSpace(s)))
			}
		}
	}
	if t := strings.TrimSpace(str(r, "text")); t != "" {
		fmt.Fprintf(w, "\n%s\n", t)
	}
	if links, ok := r["extras"].(map[string]any); ok {
		for _, key := range []string{"links", "imageLinks"} {
			if arr, ok := links[key].([]any); ok && len(arr) > 0 {
				fmt.Fprintf(w, "\n**%s:**\n", key)
				for _, l := range arr {
					fmt.Fprintf(w, "- %v\n", l)
				}
			}
		}
	}
	if subs := objects(r["subpages"]); len(subs) > 0 {
		fmt.Fprintf(w, "\n**Subpages (%d):**\n\n", len(subs))
		for i, s := range subs {
			writeResult(w, i+1, s)
		}
	}
	fmt.Fprintln(w)
}

// allFailed errors when /contents returned no page at all, so a script sees a
// non-zero exit instead of empty output.
func allFailed(raw []byte) error {
	v, err := decodeObject(raw)
	if err != nil {
		return err
	}
	if len(objects(v["results"])) == 0 && len(objects(v["statuses"])) > 0 {
		return fmt.Errorf("no URL could be fetched")
	}
	return nil
}

func oneLine(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

var fetchContents contentFlags

var fetchCmd = &cobra.Command{
	Use:   "fetch <url>...",
	Short: "Get clean page contents for one or more URLs (full text by default)",
	Example: `  exa fetch https://go.dev/doc/effective_go --max-chars 8000
  exa fetch https://example.com/a https://example.com/b --summary-query "pricing?"
  exa fetch https://docs.example.com --subpages 5 --subpage-target api,reference
  exa fetch https://news.example.com --max-age 0   # force a fresh crawl`,
	Args: cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		body, err := fetchContents.build("text")
		if err != nil {
			return err
		}
		body["urls"] = args
		ctx, cancel := reqCtx(cmd)
		defer cancel()
		raw, err := cli.Post(ctx, "/contents", body)
		if err != nil {
			return err
		}
		if err := printResults(raw); err != nil {
			return err
		}
		return allFailed(raw)
	},
}

func init() {
	fetchContents.register(fetchCmd, false)
	rootCmd.AddCommand(fetchCmd)
}

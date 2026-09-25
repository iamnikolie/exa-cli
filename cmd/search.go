package cmd

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/spf13/cobra"
)

// filterFlags are the result filters shared by search and similar.
type filterFlags struct {
	num            int
	includeDomains []string
	excludeDomains []string
	after          string
	before         string
	includeText    string
	excludeText    string
	category       string
}

func (f *filterFlags) register(cmd *cobra.Command) {
	fl := cmd.Flags()
	fl.IntVarP(&f.num, "num", "n", 10, "number of results")
	fl.StringSliceVar(&f.includeDomains, "include-domain", nil, "only these domains (repeat or comma-separate)")
	fl.StringSliceVar(&f.excludeDomains, "exclude-domain", nil, "never these domains (repeat or comma-separate)")
	fl.StringVar(&f.after, "after", "", "published on/after this date (YYYY-MM-DD or ISO 8601)")
	fl.StringVar(&f.before, "before", "", "published on/before this date (YYYY-MM-DD or ISO 8601)")
	fl.StringVar(&f.includeText, "include-text", "", "phrase (up to 5 words) that must appear in the page")
	fl.StringVar(&f.excludeText, "exclude-text", "", "phrase (up to 5 words) that must not appear in the page")
	fl.StringVar(&f.category, "category", "", "focus category: company, people, news, publication, personal site, financial report")
}

var dateOnly = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}$`)

// isoDate widens a bare YYYY-MM-DD to the ISO 8601 timestamp Exa expects.
func isoDate(s string, endOfDay bool) string {
	if !dateOnly.MatchString(s) {
		return s
	}
	if endOfDay {
		return s + "T23:59:59.999Z"
	}
	return s + "T00:00:00.000Z"
}

func (f *filterFlags) apply(body map[string]any) error {
	if len(f.includeDomains) > 0 && len(f.excludeDomains) > 0 {
		return fmt.Errorf("--include-domain and --exclude-domain are mutually exclusive")
	}
	if f.num > 0 {
		body["numResults"] = f.num
	}
	if len(f.includeDomains) > 0 {
		body["includeDomains"] = f.includeDomains
	}
	if len(f.excludeDomains) > 0 {
		body["excludeDomains"] = f.excludeDomains
	}
	if f.after != "" {
		body["startPublishedDate"] = isoDate(f.after, false)
	}
	if f.before != "" {
		body["endPublishedDate"] = isoDate(f.before, true)
	}
	if f.includeText != "" {
		body["includeText"] = []string{f.includeText}
	}
	if f.excludeText != "" {
		body["excludeText"] = []string{f.excludeText}
	}
	if f.category != "" {
		body["category"] = f.category
	}
	return nil
}

// contentFlags choose what page content comes back with each result.
type contentFlags struct {
	text           bool
	maxChars       int
	highlights     bool
	highlightQuery string
	highlightChars int
	summary        bool
	summaryQuery   string
	summarySchema  string
	noContents     bool
	maxAge         int
	livecrawl      string
	crawlTimeout   int
	subpages       int
	subpageTarget  []string
	links          int
	imageLinks     int
}

func (c *contentFlags) register(cmd *cobra.Command, withSwitch bool) {
	fl := cmd.Flags()
	fl.BoolVar(&c.text, "text", false, "include page text as markdown")
	fl.IntVar(&c.maxChars, "max-chars", 0, "cap page text at N characters (implies --text)")
	fl.BoolVar(&c.highlights, "highlights", false, "include the most relevant snippets")
	fl.StringVar(&c.highlightQuery, "highlights-query", "", "bias highlights toward this query (implies --highlights)")
	fl.IntVar(&c.highlightChars, "highlights-chars", 0, "fixed highlight budget per page (implies --highlights)")
	fl.BoolVar(&c.summary, "summary", false, "include an LLM summary of each page")
	fl.StringVar(&c.summaryQuery, "summary-query", "", "focus the summary on this question (implies --summary)")
	fl.StringVar(&c.summarySchema, "summary-schema", "", "JSON schema for a structured summary: inline, @file or - (implies --summary)")
	fl.IntVar(&c.maxAge, "max-age", -1, "max cache age in hours before a live crawl (0 = always crawl fresh)")
	fl.StringVar(&c.livecrawl, "livecrawl", "", "live-crawl policy: never, fallback, preferred, always (legacy; prefer --max-age)")
	fl.IntVar(&c.crawlTimeout, "livecrawl-timeout", 0, "live-crawl timeout in milliseconds")
	fl.IntVar(&c.subpages, "subpages", 0, "also crawl up to N subpages of each result")
	fl.StringSliceVar(&c.subpageTarget, "subpage-target", nil, "keywords that pick which subpages to crawl")
	fl.IntVar(&c.links, "links", 0, "return up to N links found on each page")
	fl.IntVar(&c.imageLinks, "image-links", 0, "return up to N image URLs found on each page")
	if withSwitch {
		fl.BoolVar(&c.noContents, "no-contents", false, "return links and metadata only (cheapest)")
	}
}

// build returns the content options. def names the content returned when the
// caller asked for none: "highlights" for search, "text" for fetch.
func (c *contentFlags) build(def string) (map[string]any, error) {
	out := map[string]any{}
	wantText := c.text || c.maxChars > 0
	wantHL := c.highlights || c.highlightQuery != "" || c.highlightChars > 0
	wantSum := c.summary || c.summaryQuery != "" || c.summarySchema != ""
	if c.noContents {
		if wantText || wantHL || wantSum {
			return nil, fmt.Errorf("--no-contents conflicts with --text/--highlights/--summary")
		}
		return nil, nil
	}
	if !wantText && !wantHL && !wantSum {
		switch def {
		case "text":
			wantText = true
		case "highlights":
			wantHL = true
		}
	}
	if wantText {
		if c.maxChars > 0 {
			out["text"] = map[string]any{"maxCharacters": c.maxChars}
		} else {
			out["text"] = true
		}
	}
	if wantHL {
		h := map[string]any{}
		if c.highlightQuery != "" {
			h["query"] = c.highlightQuery
		}
		if c.highlightChars > 0 {
			h["maxCharacters"] = c.highlightChars
		}
		if len(h) == 0 {
			out["highlights"] = true
		} else {
			out["highlights"] = h
		}
	}
	if wantSum {
		s := map[string]any{}
		if c.summaryQuery != "" {
			s["query"] = c.summaryQuery
		}
		if c.summarySchema != "" {
			schema, err := readJSONArg("summary-schema", c.summarySchema)
			if err != nil {
				return nil, err
			}
			s["schema"] = schema
		}
		if len(s) == 0 {
			out["summary"] = true
		} else {
			out["summary"] = s
		}
	}
	if c.maxAge >= 0 {
		out["maxAgeHours"] = c.maxAge
	}
	if c.livecrawl != "" {
		out["livecrawl"] = c.livecrawl
	}
	if c.crawlTimeout > 0 {
		out["livecrawlTimeout"] = c.crawlTimeout
	}
	if c.subpages > 0 {
		out["subpages"] = c.subpages
	}
	if len(c.subpageTarget) > 0 {
		out["subpageTarget"] = c.subpageTarget
	}
	if c.links > 0 || c.imageLinks > 0 {
		extras := map[string]any{}
		if c.links > 0 {
			extras["links"] = c.links
		}
		if c.imageLinks > 0 {
			extras["imageLinks"] = c.imageLinks
		}
		out["extras"] = extras
	}
	return out, nil
}

var (
	searchFilters   filterFlags
	searchContents  contentFlags
	searchType      string
	searchCountry   string
	searchSystem    string
	searchSchema    string
	searchExtraQs   []string
	searchModerate  bool
	similarFilters  filterFlags
	similarContents contentFlags
	similarNoSelf   bool
	codeNum         int
)

func buildSearchBody(query string) (map[string]any, error) {
	body := map[string]any{"query": query}
	if err := searchFilters.apply(body); err != nil {
		return nil, err
	}
	if searchType != "" {
		body["type"] = searchType
	}
	if searchCountry != "" {
		body["userLocation"] = strings.ToUpper(searchCountry)
	}
	if searchSystem != "" {
		body["systemPrompt"] = searchSystem
	}
	if searchSchema != "" {
		schema, err := readJSONArg("schema", searchSchema)
		if err != nil {
			return nil, err
		}
		body["outputSchema"] = schema
	}
	if len(searchExtraQs) > 0 {
		body["additionalQueries"] = searchExtraQs
	}
	if searchModerate {
		body["moderation"] = true
	}
	contents, err := searchContents.build("highlights")
	if err != nil {
		return nil, err
	}
	if contents != nil {
		body["contents"] = contents
	}
	return body, nil
}

var searchCmd = &cobra.Command{
	Use:   "search <query>",
	Short: "Search the web (returns results with highlights by default)",
	Example: `  exa search "rust async runtimes comparison"
  exa search "LLM eval papers" --category news --after 2026-01-01 -n 5
  exa search "who founded Exa" --type deep --schema '{"type":"text"}'
  exa search "golang release notes" --include-domain go.dev --text --max-chars 4000`,
	Args: cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		query, err := readArg(strings.Join(args, " "))
		if err != nil {
			return err
		}
		body, err := buildSearchBody(query)
		if err != nil {
			return err
		}
		ctx, cancel := reqCtx(cmd)
		defer cancel()
		raw, err := cli.Post(ctx, "/search", body)
		if err != nil {
			return err
		}
		return printResults(raw)
	},
}

var similarCmd = &cobra.Command{
	Use:   "similar <url>",
	Short: "Find pages similar to a URL",
	Example: `  exa similar https://example.com/blog/post -n 5
  exa similar https://github.com/owner/repo --exclude-source-domain --no-contents`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		body := map[string]any{"url": args[0]}
		if err := similarFilters.apply(body); err != nil {
			return err
		}
		if similarNoSelf {
			body["excludeSourceDomain"] = true
		}
		contents, err := similarContents.build("highlights")
		if err != nil {
			return err
		}
		if contents != nil {
			body["contents"] = contents
		}
		ctx, cancel := reqCtx(cmd)
		defer cancel()
		raw, err := cli.Post(ctx, "/findSimilar", body)
		if err != nil {
			return err
		}
		return printResults(raw)
	},
}

// codeCmd mirrors Exa's code-context preset: a fast search whose highlights
// are steered by the query, with a short text preview per hit.
var codeCmd = &cobra.Command{
	Use:   "code <query>",
	Short: "Search for code examples and API docs (Exa code-context preset)",
	Example: `  exa code "Go http.Client retry with exponential backoff"
  exa code "React useSyncExternalStore example" -n 5`,
	Args: cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		query, err := readArg(strings.Join(args, " "))
		if err != nil {
			return err
		}
		body := map[string]any{
			"query":      query,
			"type":       "fast",
			"numResults": codeNum,
			"contents": map[string]any{
				"highlights": map[string]any{"query": query},
				"text":       map[string]any{"maxCharacters": 300},
			},
		}
		ctx, cancel := reqCtx(cmd)
		defer cancel()
		raw, err := cli.Post(ctx, "/search", body)
		if err != nil {
			return err
		}
		return printResults(raw)
	},
}

func init() {
	searchFilters.register(searchCmd)
	searchContents.register(searchCmd, true)
	fl := searchCmd.Flags()
	fl.StringVar(&searchType, "type", "", "search type: auto (default), fast, instant, neural, deep-lite, deep, deep-reasoning")
	fl.StringVar(&searchCountry, "country", "", "two-letter country code of the user, e.g. US")
	fl.StringVar(&searchSystem, "system-prompt", "", "instructions for search planning and synthesis")
	fl.StringVar(&searchSchema, "schema", "", `synthesized output schema: {"type":"text"} or a JSON schema object; inline, @file or -`)
	fl.StringSliceVar(&searchExtraQs, "also", nil, "extra query phrasings for deep search types (repeat, max 5)")
	fl.BoolVar(&searchModerate, "moderation", false, "filter unsafe results")

	similarFilters.register(similarCmd)
	similarContents.register(similarCmd, true)
	similarCmd.Flags().BoolVar(&similarNoSelf, "exclude-source-domain", false, "drop results from the source URL's domain")

	codeCmd.Flags().IntVarP(&codeNum, "num", "n", 10, "number of results")

	rootCmd.AddCommand(searchCmd, similarCmd, codeCmd)
}

package cmd

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/iamnikolie/exa-cli/internal/client"
	"github.com/spf13/cobra"
)

var (
	answerText    bool
	answerSystem  string
	answerSchema  string
	answerModel   string
	answerCountry string
	answerStream  bool
)

func buildAnswerBody(query string) (map[string]any, error) {
	body := map[string]any{"query": query}
	if answerText {
		body["text"] = true
	}
	if answerSystem != "" {
		body["systemPrompt"] = answerSystem
	}
	if answerModel != "" {
		body["model"] = answerModel
	}
	if answerCountry != "" {
		body["userLocation"] = strings.ToUpper(answerCountry)
	}
	if answerSchema != "" {
		schema, err := readJSONArg("schema", answerSchema)
		if err != nil {
			return nil, err
		}
		body["outputSchema"] = schema
	}
	return body, nil
}

var answerCmd = &cobra.Command{
	Use:   "answer <question>",
	Short: "Answer a question with a web-grounded LLM response and citations",
	Example: `  exa answer "What is the latest stable Go version?"
  exa answer "Compare Qdrant and pgvector for 10M vectors" --stream
  exa answer "SpaceX latest valuation" --schema '{"type":"object","properties":{"valuation":{"type":"string"}}}'`,
	Args: cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		query, err := readArg(strings.Join(args, " "))
		if err != nil {
			return err
		}
		body, err := buildAnswerBody(query)
		if err != nil {
			return err
		}
		ctx, cancel := reqCtx(cmd)
		defer cancel()
		if answerStream {
			body["stream"] = true
			return streamAnswer(ctx, body)
		}
		raw, err := cli.Post(ctx, "/answer", body)
		if err != nil {
			return err
		}
		v, err := decodeObject(raw)
		if err != nil {
			return err
		}
		if format() == "json" {
			return writeJSON(stdout, v)
		}
		if f := format(); f == "table" || f == "csv" || f == "tsv" || len(fieldsFlag) > 0 {
			return emitRows(objects(v["citations"]), resultCols)
		}
		writeAnswer(stdout, v["answer"], objects(v["citations"]))
		costFooter(v)
		return nil
	},
}

func writeAnswer(w io.Writer, answer any, citations []map[string]any) {
	switch a := answer.(type) {
	case string:
		fmt.Fprintln(w, strings.TrimSpace(a))
	case nil:
	default:
		fmt.Fprintln(w, "```json")
		fmt.Fprintln(w, prettyJSON(a))
		fmt.Fprintln(w, "```")
	}
	writeSources(w, citations)
}

func writeSources(w io.Writer, citations []map[string]any) {
	if len(citations) == 0 {
		return
	}
	fmt.Fprintln(w, "\nSources:")
	for i, c := range citations {
		u := str(c, "url")
		title := oneLine(str(c, "title"))
		if title == "" || title == u {
			fmt.Fprintf(w, "[%d] %s\n", i+1, u)
			continue
		}
		fmt.Fprintf(w, "[%d] %s — %s\n", i+1, truncate(title, 120), u)
	}
}

// streamAnswer prints OpenAI-style content deltas as they arrive, then the
// citations. With --json every chunk is echoed as one JSON line instead.
func streamAnswer(ctx context.Context, body map[string]any) error {
	var citations []map[string]any
	asJSON := format() == "json"
	wrote := false
	err := cli.Stream(ctx, client.Request{Method: http.MethodPost, Path: "/answer", Body: body}, func(data []byte) error {
		if asJSON {
			_, err := fmt.Fprintf(stdout, "%s\n", data)
			return err
		}
		content, cites, err := parseAnswerChunk(data)
		if err != nil {
			return err
		}
		if content != "" {
			fmt.Fprint(stdout, content)
			wrote = true
		}
		citations = append(citations, cites...)
		return nil
	})
	if err != nil {
		return err
	}
	if !asJSON {
		if wrote {
			fmt.Fprintln(stdout)
		}
		writeSources(stdout, citations)
	}
	return nil
}

// parseAnswerChunk extracts the content delta and any citations from one
// streamed /answer chunk.
func parseAnswerChunk(data []byte) (string, []map[string]any, error) {
	v, err := decodeObject(data)
	if err != nil {
		return "", nil, err
	}
	var content string
	if choices, ok := v["choices"].([]any); ok && len(choices) > 0 {
		content = str(choices[0], "delta.content")
	}
	return content, objects(v["citations"]), nil
}

func init() {
	fl := answerCmd.Flags()
	fl.BoolVar(&answerText, "text", false, "include the full text of each citation")
	fl.StringVar(&answerSystem, "system-prompt", "", "instructions that steer the answer")
	fl.StringVar(&answerSchema, "schema", "", "JSON schema for a structured answer: inline, @file or -")
	fl.StringVar(&answerModel, "model", "", "answer model: exa (default) or exa-pro")
	fl.StringVar(&answerCountry, "country", "", "two-letter country code of the user, e.g. US")
	fl.BoolVar(&answerStream, "stream", false, "stream the answer as it is generated")
	rootCmd.AddCommand(answerCmd)
}

package cmd

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/iamnikolie/exa-cli/internal/render"
)

func format() string {
	if jsonOutput {
		return "json"
	}
	if outputFormat == "" {
		return "md"
	}
	return outputFormat
}

func decode(data []byte) (any, error) {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	var v any
	if err := dec.Decode(&v); err != nil {
		return nil, fmt.Errorf("decode Exa response: %w", err)
	}
	return v, nil
}

func decodeObject(data []byte) (map[string]any, error) {
	v, err := decode(data)
	if err != nil {
		return nil, err
	}
	m, ok := v.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("decode Exa response: expected a JSON object")
	}
	return m, nil
}

// getPath resolves a dotted path ("output.content") inside decoded JSON.
func getPath(v any, path string) (any, bool) {
	if m, ok := v.(map[string]any); ok {
		if x, ok := m[path]; ok {
			return x, true
		}
	}
	cur := v
	for _, part := range strings.Split(path, ".") {
		m, ok := cur.(map[string]any)
		if !ok {
			return nil, false
		}
		cur, ok = m[part]
		if !ok {
			return nil, false
		}
	}
	return cur, true
}

func str(v any, path string) string {
	x, ok := getPath(v, path)
	if !ok || x == nil {
		return ""
	}
	return render.Cell(x)
}

func objects(v any) []map[string]any {
	arr, _ := v.([]any)
	out := make([]map[string]any, 0, len(arr))
	for _, x := range arr {
		if m, ok := x.(map[string]any); ok {
			out = append(out, m)
		}
	}
	return out
}

func project(row map[string]any, fields []string) map[string]any {
	out := make(map[string]any, len(fields))
	for _, f := range fields {
		if v, ok := getPath(row, f); ok {
			out[f] = v
		}
	}
	return out
}

func activeFields(defaults []string) []string {
	if len(fieldsFlag) > 0 {
		return fieldsFlag
	}
	return defaults
}

// writeJSON re-encodes decoded JSON as compact UTF-8 without HTML escaping.
func writeJSON(w io.Writer, v any) error {
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	return enc.Encode(v)
}

func writeRaw(data []byte) error {
	v, err := decode(data)
	if err != nil {
		return err
	}
	return writeJSON(stdout, v)
}

func prettyJSON(v any) string {
	var b bytes.Buffer
	enc := json.NewEncoder(&b)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(v); err != nil {
		return render.Cell(v)
	}
	return strings.TrimRight(b.String(), "\n")
}

// emitRows prints a list in the active format. JSON projects each row when
// --fields is set; md and table both render a Markdown table.
func emitRows(rows []map[string]any, defaults []string) error {
	cols := activeFields(defaults)
	switch format() {
	case "json":
		if len(fieldsFlag) == 0 {
			return writeJSON(stdout, rows)
		}
		out := make([]map[string]any, len(rows))
		for i, r := range rows {
			out[i] = project(r, fieldsFlag)
		}
		return writeJSON(stdout, out)
	case "csv":
		render.Separated(stdout, projectAll(rows, cols), cols, ",")
	case "tsv":
		render.Separated(stdout, projectAll(rows, cols), cols, "\t")
	default:
		render.Table(stdout, projectAll(rows, cols), cols)
	}
	return nil
}

func projectAll(rows []map[string]any, cols []string) []map[string]any {
	out := make([]map[string]any, len(rows))
	for i, r := range rows {
		out[i] = project(r, cols)
	}
	return out
}

// costFooter reports the request cost on stderr so stdout stays clean.
func costFooter(v any) {
	if format() == "json" {
		return
	}
	total := str(v, "costDollars.total")
	if total == "" {
		return
	}
	fmt.Fprintf(stderr, "(cost $%s)\n", total)
}

func truncate(s string, n int) string {
	s = strings.Join(strings.Fields(s), " ")
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n-1]) + "…"
}

// readArg returns the literal text, or reads stdin when it is "-".
func readArg(s string) (string, error) {
	if s != "-" {
		return s, nil
	}
	b, err := io.ReadAll(stdin)
	if err != nil {
		return "", fmt.Errorf("read stdin: %w", err)
	}
	return strings.TrimSpace(string(b)), nil
}

// readJSONArg parses a JSON flag value given inline, as @file, or as - (stdin).
func readJSONArg(name, s string) (json.RawMessage, error) {
	var b []byte
	var err error
	switch {
	case s == "-":
		b, err = io.ReadAll(stdin)
	case strings.HasPrefix(s, "@"):
		b, err = os.ReadFile(s[1:])
	default:
		b = []byte(s)
	}
	if err != nil {
		return nil, fmt.Errorf("--%s: %w", name, err)
	}
	b = bytes.TrimSpace(b)
	if !json.Valid(b) {
		return nil, fmt.Errorf("--%s: not valid JSON (pass inline JSON, @file.json or - for stdin)", name)
	}
	return json.RawMessage(b), nil
}

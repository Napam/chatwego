package tools

import (
	"encoding/json"
	"fmt"
	"net/url"
	"strings"

	"github.com/openai/openai-go/v3/shared"
)

var DefineWord = register(Tool{
	Name:        "define_word",
	Description: "Looks up the dictionary definition of an English word.",
	Parameters: shared.FunctionParameters{
		"type": "object",
		"properties": map[string]any{
			"word": map[string]any{
				"type":        "string",
				"description": `The word to define, e.g. "ubiquitous".`,
			},
		},
		"required": []string{"word"},
	},
	Call: func(arguments string) (string, error) {
		var args struct {
			Word string `json:"word"`
		}
		if err := json.Unmarshal([]byte(arguments), &args); err != nil {
			return "", fmt.Errorf("parse arguments: %w", err)
		}

		u := "https://api.dictionaryapi.dev/api/v2/entries/en/" + url.PathEscape(args.Word)
		var results []struct {
			Meanings []struct {
				PartOfSpeech string `json:"partOfSpeech"`
				Definitions  []struct {
					Definition string `json:"definition"`
				} `json:"definitions"`
			} `json:"meanings"`
		}
		if err := getJSON(u, &results); err != nil {
			return "", fmt.Errorf("define %q: %w", args.Word, err)
		}
		if len(results) == 0 || len(results[0].Meanings) == 0 {
			return "", fmt.Errorf("no definition found for %q", args.Word)
		}

		var b strings.Builder
		for _, m := range results[0].Meanings {
			if len(m.Definitions) == 0 {
				continue
			}
			fmt.Fprintf(&b, "%s: %s\n", m.PartOfSpeech, m.Definitions[0].Definition)
		}
		return strings.TrimSpace(b.String()), nil
	},
})

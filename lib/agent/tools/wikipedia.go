package tools

import (
	"encoding/json"
	"fmt"
	"net/url"

	"github.com/openai/openai-go/v3/shared"
)

var Wikipedia = register(Tool{
	Name:        "search_wikipedia",
	Description: "Searches Wikipedia and returns a short summary of the best-matching article.",
	Parameters: shared.FunctionParameters{
		"type": "object",
		"properties": map[string]any{
			"query": map[string]any{
				"type":        "string",
				"description": `What to search for, e.g. "Ada Lovelace".`,
			},
		},
		"required": []string{"query"},
	},
	Call: func(arguments string) (string, error) {
		var args struct {
			Query string `json:"query"`
		}
		if err := json.Unmarshal([]byte(arguments), &args); err != nil {
			return "", fmt.Errorf("parse arguments: %w", err)
		}

		title, err := wikipediaSearch(args.Query)
		if err != nil {
			return "", fmt.Errorf("search wikipedia for %q: %w", args.Query, err)
		}
		summary, err := wikipediaSummary(title)
		if err != nil {
			return "", fmt.Errorf("fetch summary for %q: %w", title, err)
		}
		return summary, nil
	},
})

// wikipediaHeaders sets a descriptive User-Agent, as required by the
// Wikimedia API etiquette policy -- unlike the other keyless APIs here,
// Wikipedia rejects requests carrying Go's generic default client UA.
var wikipediaHeaders = map[string]string{
	"User-Agent": "chatwego (https://github.com/Napam/chatwego)",
}

// wikipediaSearch finds the best-matching article title for a free-text
// query via Wikipedia's search API, since the summary endpoint below needs an
// exact title.
func wikipediaSearch(query string) (title string, err error) {
	u := "https://en.wikipedia.org/w/api.php?action=query&list=search&format=json&srlimit=1&srsearch=" +
		url.QueryEscape(
			query,
		)
	var result struct {
		Query struct {
			Search []struct {
				Title string `json:"title"`
			} `json:"search"`
		} `json:"query"`
	}
	if err := getJSONWithHeaders(u, wikipediaHeaders, &result); err != nil {
		return "", err
	}
	if len(result.Query.Search) == 0 {
		return "", fmt.Errorf("no matching article")
	}
	return result.Query.Search[0].Title, nil
}

func wikipediaSummary(title string) (string, error) {
	u := "https://en.wikipedia.org/api/rest_v1/page/summary/" + url.PathEscape(title)
	var result struct {
		Title   string `json:"title"`
		Extract string `json:"extract"`
	}
	if err := getJSONWithHeaders(u, wikipediaHeaders, &result); err != nil {
		return "", err
	}
	if result.Extract == "" {
		return "", fmt.Errorf("no summary available")
	}
	return fmt.Sprintf("%s: %s", result.Title, result.Extract), nil
}

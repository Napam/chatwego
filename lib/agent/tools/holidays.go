package tools

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/openai/openai-go/v3/shared"
)

var PublicHolidays = register(Tool{
	Name: "get_public_holidays",
	Description: `Lists public holidays for a country in a given year. Use the ` +
		`ISO 3166-1 alpha-2 country code, e.g. "NO", "US", "GB".`,
	Parameters: shared.FunctionParameters{
		"type": "object",
		"properties": map[string]any{
			"country_code": map[string]any{
				"type":        "string",
				"description": `Two-letter ISO country code, e.g. "NO".`,
			},
			"year": map[string]any{
				"type":        "integer",
				"description": "The year to look up, e.g. 2026.",
			},
		},
		"required": []string{"country_code", "year"},
	},
	Call: func(arguments string) (string, error) {
		var args struct {
			CountryCode string `json:"country_code"`
			Year        int    `json:"year"`
		}
		if err := json.Unmarshal([]byte(arguments), &args); err != nil {
			return "", fmt.Errorf("parse arguments: %w", err)
		}

		code := strings.ToUpper(args.CountryCode)
		u := fmt.Sprintf("https://date.nager.at/api/v3/PublicHolidays/%d/%s", args.Year, code)
		var results []struct {
			Date      string `json:"date"`
			LocalName string `json:"localName"`
			Name      string `json:"name"`
		}
		if err := getJSON(u, &results); err != nil {
			return "", fmt.Errorf("fetch holidays for %s %d: %w", code, args.Year, err)
		}
		if len(results) == 0 {
			return "", fmt.Errorf("no holiday data for %s %d", code, args.Year)
		}
		var b strings.Builder
		for _, h := range results {
			fmt.Fprintf(&b, "%s: %s (%s)\n", h.Date, h.LocalName, h.Name)
		}
		return strings.TrimSpace(b.String()), nil
	},
})

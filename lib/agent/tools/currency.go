package tools

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/openai/openai-go/v3/shared"
)

var ConvertCurrency = register(Tool{
	Name: "convert_currency",
	Description: `Converts an amount from one currency to another using current exchange ` +
		`rates. Use ISO 4217 codes, e.g. "USD", "EUR", "NOK".`,
	Parameters: shared.FunctionParameters{
		"type": "object",
		"properties": map[string]any{
			"amount": map[string]any{
				"type":        "number",
				"description": "The amount to convert.",
			},
			"from": map[string]any{
				"type":        "string",
				"description": `The source currency code, e.g. "USD".`,
			},
			"to": map[string]any{
				"type":        "string",
				"description": `The target currency code, e.g. "EUR".`,
			},
		},
		"required": []string{"amount", "from", "to"},
	},
	Call: func(arguments string) (string, error) {
		var args struct {
			Amount float64 `json:"amount"`
			From   string  `json:"from"`
			To     string  `json:"to"`
		}
		if err := json.Unmarshal([]byte(arguments), &args); err != nil {
			return "", fmt.Errorf("parse arguments: %w", err)
		}

		from := strings.ToUpper(args.From)
		to := strings.ToUpper(args.To)
		u := fmt.Sprintf(
			"https://api.frankfurter.app/latest?amount=%g&from=%s&to=%s",
			args.Amount,
			from,
			to,
		)
		var result struct {
			Date  string             `json:"date"`
			Rates map[string]float64 `json:"rates"`
		}
		if err := getJSON(u, &result); err != nil {
			return "", fmt.Errorf("convert %s to %s: %w", from, to, err)
		}
		converted, ok := result.Rates[to]
		if !ok {
			return "", fmt.Errorf("no rate available for %s", to)
		}
		return fmt.Sprintf(
			"%.2f %s = %.2f %s (as of %s)",
			args.Amount,
			from,
			converted,
			to,
			result.Date,
		), nil
	},
})

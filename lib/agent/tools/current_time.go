package tools

import (
	"time"

	"github.com/openai/openai-go/v3/shared"
)

var CurrentTime = register(Tool{
	Name:        "get_current_time",
	Description: "Returns the current date and time in UTC.",
	Parameters: shared.FunctionParameters{
		"type":       "object",
		"properties": map[string]any{},
	},
	Call: func(arguments string) (string, error) {
		return time.Now().UTC().Format(time.RFC1123), nil
	},
})

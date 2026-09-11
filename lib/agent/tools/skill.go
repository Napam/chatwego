package tools

import (
	"encoding/json"
	"fmt"

	"github.com/Napam/chatwego/lib/agent/skills"
	"github.com/openai/openai-go/v3/shared"
)

var SkillTool = register(Tool{
	Name: "skill",
	Description: "Fetches the full instructions for a named skill. Available skills are " +
		"listed in the system prompt -- call this with one of those exact names before " +
		"acting on a skill that matches what the user is asking for.",
	Parameters: shared.FunctionParameters{
		"type": "object",
		"properties": map[string]any{
			"name": map[string]any{
				"type":        "string",
				"description": "The exact name of the skill to fetch, as listed in the system prompt.",
			},
		},
		"required": []string{"name"},
	},
	Call: func(arguments string) (string, error) {
		var args struct {
			Name string `json:"name"`
		}
		if err := json.Unmarshal([]byte(arguments), &args); err != nil {
			return "", fmt.Errorf("parse arguments: %w", err)
		}
		skill, ok := skills.Find(args.Name)
		if !ok {
			return "", fmt.Errorf("no skill named %q", args.Name)
		}
		return skill.Body, nil
	},
})

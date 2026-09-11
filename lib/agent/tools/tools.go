// Package tools defines the function tools the agent can call. Each tool
// lives in its own file and registers itself into All by declaring a
// package-level var initialized via register -- add a new tool by adding a
// new file, not by editing this one.
package tools

import (
	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/shared"
)

// Tool is a single function tool: its OpenAI-facing definition
// (name/description/parameters) plus the Go function that executes it.
// Call receives the model's raw JSON arguments string (as sent by the API)
// and returns the result to feed back to the model.
type Tool struct {
	Name        string
	Description string
	Parameters  shared.FunctionParameters
	Call        func(arguments string) (string, error)
}

// Param builds the param used to advertise this tool in a chat completion
// request's Tools list.
func (t Tool) Param() openai.ChatCompletionToolUnionParam {
	return openai.ChatCompletionFunctionTool(shared.FunctionDefinitionParam{
		Name:        t.Name,
		Description: openai.String(t.Description),
		Parameters:  t.Parameters,
	})
}

// All is the registry of every tool the agent can call, in registration
// order.
var All []Tool

func register(t Tool) Tool {
	All = append(All, t)
	return t
}

// Find looks up a registered tool by name.
func Find(name string) (Tool, bool) {
	for _, t := range All {
		if t.Name == name {
			return t, true
		}
	}
	return Tool{}, false
}

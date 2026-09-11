package tools

import (
	"encoding/json"
	"fmt"

	"github.com/openai/openai-go/v3/shared"
)

var GitHubRepo = register(Tool{
	Name: "get_github_repo",
	Description: `Gets public info about a GitHub repository (stars, description, ` +
		`language, open issues). Takes "owner/repo", e.g. "golang/go".`,
	Parameters: shared.FunctionParameters{
		"type": "object",
		"properties": map[string]any{
			"repo": map[string]any{
				"type":        "string",
				"description": `The repository in "owner/name" form, e.g. "golang/go".`,
			},
		},
		"required": []string{"repo"},
	},
	Call: func(arguments string) (string, error) {
		var args struct {
			Repo string `json:"repo"`
		}
		if err := json.Unmarshal([]byte(arguments), &args); err != nil {
			return "", fmt.Errorf("parse arguments: %w", err)
		}

		var result struct {
			FullName    string `json:"full_name"`
			Description string `json:"description"`
			Language    string `json:"language"`
			Stars       int    `json:"stargazers_count"`
			OpenIssues  int    `json:"open_issues_count"`
			HTMLURL     string `json:"html_url"`
		}
		// GitHub's API rejects requests with no User-Agent header, unlike the
		// other keyless APIs here -- getJSONWithHeaders exists for this.
		headers := map[string]string{
			"Accept":     "application/vnd.github+json",
			"User-Agent": "chatwego",
		}
		if err := getJSONWithHeaders(
			"https://api.github.com/repos/"+args.Repo,
			headers,
			&result,
		); err != nil {
			return "", fmt.Errorf("fetch repo %q: %w", args.Repo, err)
		}
		if result.FullName == "" {
			return "", fmt.Errorf("no repository found for %q", args.Repo)
		}
		return fmt.Sprintf(
			"%s: %s (language: %s, stars: %d, open issues: %d) %s",
			result.FullName, result.Description, result.Language,
			result.Stars, result.OpenIssues, result.HTMLURL,
		), nil
	},
})

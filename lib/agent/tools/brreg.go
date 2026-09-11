package tools

import (
	"encoding/json"
	"fmt"
	"net/url"
	"regexp"
	"strings"

	"github.com/openai/openai-go/v3/shared"
)

var orgNumberPattern = regexp.MustCompile(`^\d{9}$`)

var BrregSearch = register(Tool{
	Name: "search_brreg",
	Description: `Looks up a company in Brønnøysundregistrene, Norway's central business ` +
		`registry. Takes either a company name or a 9-digit organisasjonsnummer (org number).`,
	Parameters: shared.FunctionParameters{
		"type": "object",
		"properties": map[string]any{
			"query": map[string]any{
				"type": "string",
				"description": `A company name (e.g. "Telenor") or a 9-digit org number ` +
					`(e.g. "982463718").`,
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

		query := strings.TrimSpace(args.Query)
		if orgNumberPattern.MatchString(query) {
			entity, err := brregByOrgNumber(query)
			if err != nil {
				return "", fmt.Errorf("look up org number %q: %w", query, err)
			}
			return formatBrregEntity(entity), nil
		}

		entities, err := brregSearchByName(query)
		if err != nil {
			return "", fmt.Errorf("search brreg for %q: %w", query, err)
		}
		if len(entities) == 0 {
			return "", fmt.Errorf("no company found for %q", query)
		}
		lines := make([]string, len(entities))
		for i, e := range entities {
			lines[i] = formatBrregEntity(e)
		}
		return strings.Join(lines, "\n"), nil
	},
})

// brregEntity covers the fields of an Enhetsregisteret "enhet" that are
// useful to surface, out of the much larger set the API actually returns.
type brregEntity struct {
	OrgNumber string `json:"organisasjonsnummer"`
	Name      string `json:"navn"`
	OrgForm   struct {
		Description string `json:"beskrivelse"`
	} `json:"organisasjonsform"`
	IndustryCode struct {
		Description string `json:"beskrivelse"`
	} `json:"naeringskode1"`
	BusinessAddress struct {
		Municipality string `json:"kommune"`
	} `json:"forretningsadresse"`
	Employees  int  `json:"antallAnsatte"`
	Bankrupt   bool `json:"konkurs"`
	Dissolving bool `json:"underAvvikling"`
}

func brregByOrgNumber(orgNumber string) (brregEntity, error) {
	var entity brregEntity
	u := "https://data.brreg.no/enhetsregisteret/api/enheter/" + url.PathEscape(orgNumber)
	if err := getJSON(u, &entity); err != nil {
		return brregEntity{}, err
	}
	return entity, nil
}

func brregSearchByName(name string) ([]brregEntity, error) {
	u := "https://data.brreg.no/enhetsregisteret/api/enheter?size=5&navn=" + url.QueryEscape(name)
	var result struct {
		Embedded struct {
			Enheter []brregEntity `json:"enheter"`
		} `json:"_embedded"`
	}
	if err := getJSON(u, &result); err != nil {
		return nil, err
	}
	return result.Embedded.Enheter, nil
}

func formatBrregEntity(e brregEntity) string {
	status := "active"
	switch {
	case e.Bankrupt:
		status = "bankrupt"
	case e.Dissolving:
		status = "being dissolved"
	}
	return fmt.Sprintf(
		"%s (org.nr %s): %s, industry: %s, municipality: %s, employees: %d, status: %s",
		e.Name, e.OrgNumber, e.OrgForm.Description, e.IndustryCode.Description,
		e.BusinessAddress.Municipality, e.Employees, status,
	)
}

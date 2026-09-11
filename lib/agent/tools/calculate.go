package tools

import (
	"encoding/json"
	"fmt"
	"strconv"

	"github.com/openai/openai-go/v3/shared"
)

var Calculate = register(Tool{
	Name: "calculate",
	Description: "Evaluates a basic arithmetic expression (+, -, *, /, parentheses) and " +
		"returns the result. Use this for any nontrivial math instead of computing it yourself.",
	Parameters: shared.FunctionParameters{
		"type": "object",
		"properties": map[string]any{
			"expression": map[string]any{
				"type":        "string",
				"description": `An arithmetic expression, e.g. "(3 + 4) * 2".`,
			},
		},
		"required": []string{"expression"},
	},
	Call: func(arguments string) (string, error) {
		var args struct {
			Expression string `json:"expression"`
		}
		if err := json.Unmarshal([]byte(arguments), &args); err != nil {
			return "", fmt.Errorf("parse arguments: %w", err)
		}
		result, err := evalArithmetic(args.Expression)
		if err != nil {
			return "", fmt.Errorf("evaluate %q: %w", args.Expression, err)
		}
		return strconv.FormatFloat(result, 'g', -1, 64), nil
	},
})

// arithmeticParser is a minimal recursive-descent parser for arithmetic
// expressions with +, -, *, /, unary minus, and parentheses -- deliberately
// hand-rolled rather than an eval() of some kind, since the expression comes
// from model output and must never reach a real code evaluator.
type arithmeticParser struct {
	expr string
	pos  int
}

func evalArithmetic(expr string) (float64, error) {
	p := &arithmeticParser{expr: expr}
	val, err := p.parseExpr()
	if err != nil {
		return 0, err
	}
	p.skipSpace()
	if p.pos != len(p.expr) {
		return 0, fmt.Errorf("unexpected character %q at position %d", p.expr[p.pos], p.pos)
	}
	return val, nil
}

func (p *arithmeticParser) skipSpace() {
	for p.pos < len(p.expr) && p.expr[p.pos] == ' ' {
		p.pos++
	}
}

func (p *arithmeticParser) parseExpr() (float64, error) {
	val, err := p.parseTerm()
	if err != nil {
		return 0, err
	}
	for {
		p.skipSpace()
		if p.pos >= len(p.expr) {
			return val, nil
		}
		switch p.expr[p.pos] {
		case '+':
			p.pos++
			rhs, err := p.parseTerm()
			if err != nil {
				return 0, err
			}
			val += rhs
		case '-':
			p.pos++
			rhs, err := p.parseTerm()
			if err != nil {
				return 0, err
			}
			val -= rhs
		default:
			return val, nil
		}
	}
}

func (p *arithmeticParser) parseTerm() (float64, error) {
	val, err := p.parseFactor()
	if err != nil {
		return 0, err
	}
	for {
		p.skipSpace()
		if p.pos >= len(p.expr) {
			return val, nil
		}
		switch p.expr[p.pos] {
		case '*':
			p.pos++
			rhs, err := p.parseFactor()
			if err != nil {
				return 0, err
			}
			val *= rhs
		case '/':
			p.pos++
			rhs, err := p.parseFactor()
			if err != nil {
				return 0, err
			}
			if rhs == 0 {
				return 0, fmt.Errorf("division by zero")
			}
			val /= rhs
		default:
			return val, nil
		}
	}
}

func (p *arithmeticParser) parseFactor() (float64, error) {
	p.skipSpace()
	if p.pos >= len(p.expr) {
		return 0, fmt.Errorf("unexpected end of expression")
	}
	if p.expr[p.pos] == '-' {
		p.pos++
		val, err := p.parseFactor()
		if err != nil {
			return 0, err
		}
		return -val, nil
	}
	if p.expr[p.pos] == '(' {
		p.pos++
		val, err := p.parseExpr()
		if err != nil {
			return 0, err
		}
		p.skipSpace()
		if p.pos >= len(p.expr) || p.expr[p.pos] != ')' {
			return 0, fmt.Errorf("missing closing parenthesis")
		}
		p.pos++
		return val, nil
	}

	start := p.pos
	for p.pos < len(p.expr) && (isDigit(p.expr[p.pos]) || p.expr[p.pos] == '.') {
		p.pos++
	}
	if start == p.pos {
		return 0, fmt.Errorf("unexpected character %q at position %d", p.expr[p.pos], p.pos)
	}
	val, err := strconv.ParseFloat(p.expr[start:p.pos], 64)
	if err != nil {
		return 0, fmt.Errorf("invalid number %q", p.expr[start:p.pos])
	}
	return val, nil
}

func isDigit(b byte) bool {
	return b >= '0' && b <= '9'
}

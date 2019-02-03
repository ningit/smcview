package maude

import (
	"regexp"
	"strconv"
	"strings"
)

// Constants and regular expressions for parsing Maude output
var noParseRegex = regexp.MustCompile("^no(?:Strat)?Parse\\(([^\\)]+)\\)$")

// ParseOutcome represents the possible outcomes of a parsing operation.
type ParseOutcome int

const (
	Ok ParseOutcome = iota
	GenError
	NoParse
	Ambiguity
)

// ParseResult represents a parsing operation result, where Pos tell the
// position in case of a syntax error.
type ParseResult struct {
	Type ParseOutcome
	Pos  int
}

// Parse tries to parse a term of the given sort in the current module.
func (c *Client) Parse(term, sort string) ParseResult {

	if !c.active {
		return ParseResult{Type: GenError}
	}

	// The function should not change the current module
	var module = c.CurrentModuleName()
	defer c.Select(module)

	// Tokenize the given term
	result := c.ReduceIn("LEXICAL", "tokenize(\""+term+"\")")

	if !result.Ok {
		return ParseResult{Type: GenError}
	}

	// Parse the tokenized term in the original module
	result = c.ReduceIn("META-LEVEL", "metaParse(upModule('"+module+
		", false), "+result.Term+", '"+sort+")")

	if !result.Ok {
		return ParseResult{Type: GenError}
	}

	// The parse was not successful if its type is ResultPair?
	if result.Type == "ResultPair?" {

		if strings.HasPrefix(result.Term, "ambiguity") {
			return ParseResult{Type: Ambiguity}
		}

		if match := noParseRegex.FindStringSubmatch(result.Term); match != nil {
			pos, _ := strconv.Atoi(match[1])
			return ParseResult{NoParse, pos}
		}

		return ParseResult{Type: GenError}
	}

	return ParseResult{Type: Ok}
}

// StratParse tries a strategy in the current module.
func (c *Client) StratParse(expr string) ParseResult {

	if !c.active {
		return ParseResult{Type: GenError}
	}

	// The function should not alter the current module
	var module = c.CurrentModuleName()
	defer c.Select(module)

	// Tokenize the given expression
	result := c.ReduceIn("LEXICAL", "tokenize(\""+expr+"\")")

	if !result.Ok {
		return ParseResult{Type: GenError}
	}

	// Parse the tokenized expression in the original module
	result = c.ReduceIn("META-LEVEL", "metaStratParse(upModule('"+module+
		", false), "+result.Term+")")

	if !result.Ok {
		return ParseResult{Type: GenError}
	}

	// The parse was not successful if its type is Strategy?
	if result.Type == "Strategy?" {

		if strings.HasPrefix(result.Term, "ambiguity") {
			return ParseResult{Type: Ambiguity}
		}

		if match := noParseRegex.FindStringSubmatch(result.Term); match != nil {
			pos, _ := strconv.Atoi(match[1])
			return ParseResult{NoParse, pos}
		}

		return ParseResult{Type: GenError}
	}

	return ParseResult{Type: Ok}
}

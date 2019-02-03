package maude

import (
	"regexp"
	"strings"
)

// Constants and regular expressions for parsing Maude output
var resultRegex  = regexp.MustCompile("^result ([^:]+): (.*)\n$")

// ReduceResult describes the result of a reduction in Maude.
type ReduceResult struct {
	Ok   bool
	Term string
	Type string
}

// Reduce reduces a term in the current module.
func (c *Client) Reduce(term string) ReduceResult {
	return c.reduce("red " + term + " .\n")
}

// Reduce reduces a term in the given module.
func (c *Client) ReduceIn(module, term string) ReduceResult {
	return c.reduce("red in " + module + " : " + term + " .\n")
}

func (c *Client) reduce(command string) ReduceResult {
	var result = ReduceResult{Ok: false}

	if !c.active {
		return result
	}

	// At the moment, the rewriting count and similar data is ignored
	c.stdin.Write([]byte(command))

	for !c.promptReached() {
		str, _ := c.stdout.ReadString('\n')

		var match = resultRegex.FindStringSubmatch(str)

		if match != nil {
			result.Ok = true
			result.Type = match[1]
			result.Term = match[2]
			break
		}
	}

	// Terms can span multiple lines because of format
	if !c.promptReached() {
		result.Term += "\n"

		for !c.promptReached() {
			line, _ := c.stdout.ReadString('\n')
			result.Term += line
		}

		result.Term = strings.TrimSuffix(result.Term, "\n")
	}

	c.advanceUntilPrompt()

	return result
}

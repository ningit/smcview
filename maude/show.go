package maude

import (
	"regexp"
	"strings"
)

// Constants and regular expressions for parsing Maude output
var (
	moddeclRegex = regexp.MustCompile("^(fmod|mod|smod|fth|th|sth) ([^ {]+)(?:{([^}]*)})? is\n$")
	sortRegex    = regexp.MustCompile("^sort ([^ ]+) .")
	stratRegex   = regexp.MustCompile("^strat ([^ ]+)(?: : ([^@]+))? @ ([^ ]+)")
	propRegex    = regexp.MustCompile("^op ([^ ]+) : (.*)-> ([^ ]+)")
)


// ModuleInfo describes a Maude module or theory by its name and type
// (fmod, mod, smod, fth, th or sth).
type ModuleInfo struct {
	Name string `json:"name"`
	Type string `json:"type"`
}

// ExtendedModuleInfo describes a Maude module with its parameter theories.
type ExtendedModuleInfo struct {
	ModuleInfo
	Params []string `json:"params"`
}

// Modules returns all modules and theories defined in the current Maude
// session. Instantiated and renamed modules are ignored.
func (c *Client) Modules() []ModuleInfo {
	if !c.active {
		return nil
	}

	c.stdin.Write([]byte("show modules .\n"))

	var modules = make([]ModuleInfo, 0)

	for !c.promptReached() {
		str, _ := c.stdout.ReadString('\n')

		if match := modRegex.FindStringSubmatch(str); match != nil {
			modules = append(modules, ModuleInfo{match[2], match[1]})
		}
	}

	c.advanceUntilPrompt()

	return modules
}

// GetModInfo provides information about a module including
// its parameter theories.
func (c *Client) GetModInfo(name string) ExtendedModuleInfo {
	var modinfo = ExtendedModuleInfo{ModuleInfo: ModuleInfo{Name: name}}

	if !c.active {
		return modinfo
	}

	c.stdin.Write([]byte("show module " + name + " .\n"))
	str, _ := c.stdout.ReadString('\n')

	if match := moddeclRegex.FindStringSubmatch(str); match != nil {
		modinfo.Type = match[1]

		if match[3] == "" {
			// In case there is no braces
			modinfo.Params = make([]string, 0)
		} else {
			modinfo.Params = strings.Split(match[3], ", ")

			for i, value := range modinfo.Params {
				// Drops the parameter name to store only the theory name
				var index = strings.LastIndex(value, " :: ") + 4
				modinfo.Params[i] = value[index:]
			}
		}
	}

	c.advanceUntilPrompt()

	return modinfo
}

// Sorts returns all sorts defined in the current modules and its imports.
func (c *Client) Sorts() []string {
	if !c.active {
		return nil
	}

	c.stdin.Write([]byte("show sorts .\n"))

	var sorts = make([]string, 0)

	for !c.promptReached() {
		str, _ := c.stdout.ReadString('\n')

		var match = sortRegex.FindStringSubmatch(str)
		sorts = append(sorts, match[1])
	}

	c.advanceUntilPrompt()

	return sorts
}

// Subsorts returns all sub- and supersorts of a given sort
// in the current module.
func (c *Client) Subsorts(sort string) ([]string, []string) {
	if !c.active {
		return nil, nil
	}

	c.stdin.Write([]byte("show sorts .\n"))
	defer c.advanceUntilPrompt()

	// The command show sorts produces lines of the form:
	//	sort S . subsorts sub < S < sup .
	// The sub and sup parts or the whole subsorts block
	// are omitted when empty.
	var prefix = "sort " + sort + " ."

	for !c.promptReached() {
		str, _ := c.stdout.ReadString('\n')

		// The required sort was found
		if strings.HasPrefix(str, prefix) {

			var sub = make([]string, 0)
			var super = make([]string, 0)

			// There is a subsort declaration
			if len(str) > len(prefix)+10 {
				// Only the subsort part is retained
				str = str[len(prefix)+9 : len(str)-2]

				// Look for the < signs as anchors
				var less = strings.IndexByte(str, '<')
				var sortIndex = strings.Index(str, " "+sort+" ")
				var greater = strings.LastIndexByte(str, '<')

				if less < sortIndex {
					sub = strings.Split(str[:less-1], " ")
				}

				if greater > sortIndex {
					super = strings.Split(str[greater+2:], " ")
				}
			}

			return sub, super
		}
	}

	// The sort does not exist
	return nil, nil
}

// NamedStrategy describes a strategy declaration with its name,
// parameter sorts and subject sort.
type NamedStrategy struct {
	Name        string
	Params      []string
	SubjectSort string
}

// Strategies returns all strategies defined in the current module and
// its imports.
func (c *Client) Strategies() []NamedStrategy {
	if !c.active {
		return nil
	}

	c.stdin.Write([]byte("show strats .\n"))

	var strats = make([]NamedStrategy, 0)

	for !c.promptReached() {
		str, _ := c.stdout.ReadString('\n')

		if match := stratRegex.FindStringSubmatch(str); match != nil {
			var params = make([]string, 0)

			if len(match[2]) > 0 {
				params = strings.Split(match[2], " ")
			}

			strats = append(strats, NamedStrategy{match[1], params, match[3]})
		}
	}

	c.advanceUntilPrompt()

	return strats
}

// MaudeOperator describes the signature of an operator.
type MaudeOperator struct {
	Name   string
	Params []string
	Range  string
}

// AtomicProps returns all atomic propositions (i.e. all operator whose range
// sort is Prop or a subsort) defined in the current module and its imports.
func (c *Client) AtomicProps() []MaudeOperator {
	if !c.active {
		return nil
	}

	// The user might have defined atomic propositions in a custom subsort
	// of Prop. Hence, propSorts will act as the set of all Prop sorts and
	// all operator with a range in the set will be collected.
	var propSorts = make(map[string]struct{})
	propSorts["Prop"] = struct{}{}

	subsorts, _ := c.Subsorts("Prop")

	// Prop is not defined, we signal it by returning nil
	if subsorts == nil {
		return nil
	}

	for _, value := range subsorts {
		propSorts[value] = struct{}{}
	}

	c.stdin.Write([]byte("show op .\n"))

	var props = make([]MaudeOperator, 0)

	for !c.promptReached() {
		str, _ := c.stdout.ReadString('\n')

		if match := propRegex.FindStringSubmatch(str); match != nil {

			// If the range sort is that of an atomic proposition
			if _, isProp := propSorts[match[3]]; isProp {
				var params []string

				if match[2] == "" {
					params = make([]string, 0)
				} else {
					params = strings.Split(strings.TrimRight(match[2], " "), " ")
				}

				props = append(props, MaudeOperator{match[1], params, "Prop"})
			}
		}
	}

	c.advanceUntilPrompt()

	return props
}

// collectStatements collects all statements in the current module starting with the
// given keyword (an its conditional version). If the argument is a non-empty string,
// only statements with that label will be listed.
func (c *Client) collectStatements(statementType, keyword, label string) []string {
	if !c.active {
		return nil
	}

	c.stdin.Write([]byte("show " + statementType + " ."))

	var statements = make([]string, 0)
	// Label detection may not be accurate
	var labelAttribute = "label " + label
	// Conditional keyword are prefixed by a c
	keyword = keyword + " "
	var conditionalKeyword = "c" + keyword
	// A statement may continue in multiple lines we have to collect
	var statement = ""

	// Function to append statements after checking they have the right label
	var appendStatement = func (statement string) {
		statement = strings.TrimSuffix(statement, "\n")
		if label == "" || strings.Contains(statement, labelAttribute) {
			statements = append(statements, statement)
		}
	}

	for !c.promptReached() {
		line, _ := c.stdout.ReadString('\n')

		if strings.HasPrefix(line, conditionalKeyword) || strings.HasPrefix(line, keyword) {
			// A new rule starts, the old one has to be saved
			if statement != "" {
				appendStatement(statement)
				statement = line
			}
		} else {
			// Another line for the same statement
			statement = statement + line
		}
	}

	if statement != "" {
		appendStatement(statement)
	}

	c.advanceUntilPrompt()

	return statements
}

// Rules returns all rule statements in the current module. If the argument is
// a non-empty string, only rules with that label will be listed.
func (c *Client) Rules(label string) []string {
	return c.collectStatements("rules", "rl", label)
}

// Equations returns all equation statements in the current module. If the
// argument is a non-empty string, only equations with that label will be listed.
func (c *Client) Equations(label string) []string {
	return c.collectStatements("eqs", "eq", label)
}

// Memberships returns all membership axiom statements in the
// current module. If the argument is a non-empty string, only axioms
// with that label will be listed.
func (c *Client) Memberships(label string) []string {
	return c.collectStatements("mbs", "mb", label)
}

// StrategyDefinitions returns all strategy definition statements in the
// current module. If the argument is a non-empty string, only definitions
// with that label will be listed.
func (c *Client) StrategyDefinitions(label string) []string {
	return c.collectStatements("sds", "sd", label)
}

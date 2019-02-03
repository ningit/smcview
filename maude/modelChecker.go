package maude

import (
	"os"
	"strings"
)

// SetSmcOutput enables and sets the extended output path for the
// strategy-aware model checker. An empty string disables such extended
// output. For the change to take effect, Start must be called afterwards.
func (c *Client) SetSmcOutput(path string) {
	if c.command.Env == nil {
		c.command.Env = os.Environ()
	}

	for i := len(c.command.Env) - 1; i >= 0; i-- {
		if strings.HasPrefix(c.command.Env[i], "MAUDE_SMC_OUTPUT=") {
			c.command.Env[i] = "MAUDE_SMC_OUTPUT=" + path
			return
		}
	}

	// Only if not found within the environment
	c.command.Env = append(c.command.Env,
		"MAUDE_SMC_OUTPUT="+path,
	)
}

// SmcAvailable checks if the strategy model checker is available
// in the current module.
func (c *Client) SmcAvailable() bool {
	if !c.active {
		return false
	}

	c.stdin.Write([]byte("show op .\n"))
	defer c.advanceUntilPrompt()

	for !c.promptReached() {
		str, _ := c.stdout.ReadString('\n')

		if str == "    id-hook StrategyModelCheckerSymbol\n" {
			return true
		}
	}

	return false
}

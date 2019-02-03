// Package maude provides access to the Maude interpreter.
package maude

import (
	"bufio"
	"bytes"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// Constants and regular expressions for parsing Maude output
var (
	maudePrompt  = []byte("Maude> ")
	promptLength = len(maudePrompt)

	modRegex     = regexp.MustCompile("^(fmod|mod|smod|fth|th|sth) ([^ {]+)\n$")
)

// Client is an access point for the Maude interpreter.
type Client struct {
	maudePath string
	command   *exec.Cmd
	stdin     io.WriteCloser
	stdout    *bufio.Reader
	active    bool
}

// InitMaude creates a Maude client.
func InitMaude(path string) *Client {

	var client = Client{maudePath: path}

	client.initInternal()

	return &client
}

func consoleLogger(reader io.ReadCloser) {
	buffered := bufio.NewReader(reader)

	for {
		str, err := buffered.ReadString('\n')

		// Probably program termination
		if err != nil {
			return
		}

		print("### ", str)
	}
}

func (client *Client) initInternal() {

	// Preserves the environment variables between consecutive executions
	var oldEnviron []string = nil

	if client.command != nil {
		oldEnviron = client.command.Env
	}

	client.command = exec.Command(client.maudePath, "-no-banner",
		"-no-advise", "-no-wrap", "-no-ansi-color",
		"-no-tecla", "-interactive")

	if oldEnviron != nil {
		client.command.Env = oldEnviron
	}

	// Communication with Maude is based on pipes
	client.stdin, _ = client.command.StdinPipe()
	stdout, _ := client.command.StdoutPipe()
	stderr, _ := client.command.StderrPipe()

	client.stdout = bufio.NewReader(stdout)

	// The standard error is printed to the terminal by a goroutine
	go consoleLogger(stderr)

	client.active = false
}

// Load loads a source file within the Maude interpreter.
func (c *Client) Load(source string) bool {
	if !c.active {
		return false
	}

	c.stdin.Write([]byte("load " + source + " .\n"))
	c.advanceUntilPrompt()
	return true
}

// Quit politely quits from the Maude interpreter.
func (c *Client) Quit() {
	if c.active {
		c.stdin.Write([]byte("quit .\n"))
		c.command.Wait()
		c.active = false
	}
}

// Kill terminates the interpreter process.
func (c *Client) Kill() {
	if c.active {
		c.command.Process.Kill()
		c.active = false
	}
}

// QuitTimeout tries to quit the interpreter politely, but if it does not quit
// in the specified timeout (in milliseconds), the interpreter process is killed.
func (c *Client) QuitTimeout(msec int) bool {
	if !c.active {
		return true
	}

	// Channel to notify that the process has quit
	chn := make(chan struct{}, 1)

	go func() {
		c.Quit()
		chn <- struct{}{}
	}()

	select {
	case <-chn:
		return true
	// If in msec time the process Quit has not succeeded,
	// the process is killed
	case <-time.After(time.Duration(msec) * time.Millisecond):
		c.Kill()
		return false
	}
}

// Start runs a new fresh session of the Maude interpreter. It can be called
// several times; if the client is still active, it will be quit.
func (c *Client) Start() {
	if c.active {
		c.QuitTimeout(1000)
	}

	c.initInternal()
	c.command.Start()
	c.active = true
	c.advanceUntilPrompt()
}

// CurrentModuleName gets the name of the current module for the interpreter.
func (c *Client) CurrentModuleName() string {
	if !c.active {
		return ""
	}

	c.stdin.Write([]byte("show module .\n"))
	defer c.advanceUntilPrompt()

	if !c.promptReached() {
		str, _ := c.stdout.ReadString('\n')

		if match := moddeclRegex.FindStringSubmatch(str); match != nil {
			return match[2]
		}
	}

	return ""
}

// Select selects a module in the Maude interpreter.
func (c *Client) Select(module string) {
	if c.active {
		c.stdin.Write([]byte("select " + module + " .\n"))
		c.advanceUntilPrompt()
	}
}

// SetMixfix enables or disables printing in mixfix syntax.
func (c *Client) SetMixfix(value bool) {
	if !c.active {
		return
	}

	switch value {
	case true:
		c.stdin.Write([]byte("set print mixfix on .\n"))
	case false:
		c.stdin.Write([]byte("set print mixfix off .\n"))
	}

	c.advanceUntilPrompt()
}

// RawInput intoduces raw input (followed by a line break) to the Maude
// interpreter and returns its output.
func (c *Client) RawInput(input string) string {
	if !c.active {
		return "inactive"
	}

	var output strings.Builder

	c.stdin.Write([]byte(input + "\n"))

	for !c.promptReached() {
		str, _ := c.stdout.ReadString('\n')
		output.WriteString(str)
	}

	c.advanceUntilPrompt()

	return output.String()
}

func (c *Client) promptReached() bool {
	prompt, _ := c.stdout.Peek(promptLength)

	return bytes.Equal(maudePrompt, prompt)
}

func (c *Client) advanceUntilPrompt() {
	prompt, _ := c.stdout.Peek(promptLength)

	for !bytes.Equal(maudePrompt, prompt) {
		c.stdout.ReadString('\n')
		prompt, _ = c.stdout.Peek(promptLength)
	}

	c.stdout.Discard(len(maudePrompt))
}

// LocateMaude looks for the Maude executable in the host system, returning
// its path and version. Only versions with strategy support are admitted.
// If it cannot be found, empty strings are returned.
//
// The following paths are tried in order: a binary named "maude" in the
// working directory, a binary named "maude" in the directory of this
// program's binary, the environment variable "SMAUDE", and the program
// "maude" in the system path.
func LocateMaude() (string, string) {
	path, _ := filepath.Abs("maude")

	if ok, version := checkMaude(path); ok {
		return path, version
	}

	path, err := os.Executable()

	if err != nil {
		path = filepath.Join(filepath.Dir(path), "maude")

		if ok, version := checkMaude(path); ok {
			return path, version
		}
	}

	path = os.Getenv("SMAUDE")

	if ok, version := checkMaude(path); ok {
		return path, version
	}

	path, err = exec.LookPath("maude")

	if err != nil {
		path, _ := filepath.Abs(path)

		if ok, version := checkMaude(path); ok {
			return path, version
		}
	}

	return "", ""
}

// CheckMaude checks that the executable in path is a Maude
// interpreter with strategy support.
func checkMaude(path string) (bool, string) {
	fileinfo, _ := os.Stat(path)

	if fileinfo != nil && !fileinfo.IsDir() {
		var version = MaudeVersion(path)

		if strings.Contains(version, "+strat") {
			return true, version
		}
	}

	return false, ""
}

// MaudeVersion returns the version of the Maude executable pointed
// by the given path. An empty string is returned on any error.
func MaudeVersion(path string) string {
	var cmd = exec.Command(path, "--version")

	output, _ := cmd.Output()
	cmd.Run()

	if output == nil {
		return ""
	}

	line, _, _ := bufio.NewReader(bytes.NewReader(output)).ReadLine()

	return string(line)
}

// +build linux openbsd freebsd netbsd

package webui

import "os/exec"

func openBrowser(url string) {
	exec.Command("xdg-open", url).Run()
}

// +build !windows

package webui

import (
	"path"
	"path/filepath"
	"strings"
)

// web2NativeUrl translate a URL transmitted to the browser into a file
// location in the host machine. This is used to allow restricting access
// to a given root. The second return value is a Boolean that will be true
// whenever the given URL does not refer to a host file or directory but
// to a special directory (used only for accessing to the logical volumes
// in Windows).
func (s *WebUi) web2NativeUrl(webUrl string) (string, bool) {
	webUrl = path.Clean(webUrl)

	// Attempt to access outside the root directory
	if strings.Contains(webUrl, "..") {
		return "", false
	}

	return filepath.Join(s.RootDir, webUrl), false
}

// native2WebUrl translates a native URL to one suitable to be transmitted
// to the browser.
func (s *WebUi) native2WebUrl(nativeUrl string) string {
	if s.RootDir != "" {
		nativeUrl, _ := filepath.Rel(s.RootDir, nativeUrl)
		return path.Join("/", nativeUrl)
	}

	return nativeUrl
}

// specialUrl returns the list of directories and files of a given
// special directory (not used but in Windows).
func (s *WebUi) specialUrl(webUrl string) ([]string, []string) {
	return nil, nil
}

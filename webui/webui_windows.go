package webui

import (
	"path"
	"path/filepath"
	"os/exec"
	"strings"
	"syscall"
)

func (s *WebUi) web2NativeUrl(webUrl string) (string, bool) {
	webUrl = path.Clean(webUrl)

	// Attempt to access outside the root directory
	if strings.Contains(webUrl, "..") {
		return "", false
	}

	if s.RootDir == "" {
		if webUrl == "/" {
			// The root web url is the volume list in Windows
			return webUrl, true
		}

		var dirs = strings.Split(webUrl, "/")
		dirs[1] = strings.ToUpper(dirs[1]) + string(filepath.Separator)
		return filepath.Join(dirs[1:]...), false
	}

	return filepath.Join(s.RootDir, filepath.FromSlash(webUrl)), false
}

func (s *WebUi) native2WebUrl(nativeUrl string) string {
	if s.RootDir != "" {
		nativeUrl, _ := filepath.Rel(s.RootDir, nativeUrl)
		return filepath.Join("/", filepath.ToSlash(nativeUrl))
	}

	if !filepath.IsAbs(nativeUrl) {
		return filepath.ToSlash(nativeUrl)
	}

	// We convert the volume name to a directory name
	var volumeName = filepath.VolumeName(nativeUrl)
	var rest = strings.TrimPrefix(nativeUrl, volumeName)

	return path.Join("/", strings.ToLower(volumeName), filepath.ToSlash(rest))
}

func (s *WebUi) specialUrl(webUrl string) ([]string, []string) {
	kernel32, _  := syscall.LoadLibrary("kernel32.dll")
	gldHandle, _ := syscall.GetProcAddress(kernel32, "GetLogicalDrives")

	var drives = make([]string, 0)

	if bitMap, _, err := syscall.Syscall(uintptr(gldHandle), 0, 0, 0, 0); err == 0 {
		for i := 0; i < 26; i++ {
			if bitMap & 1 == 1 {
				drives = append(drives, string('A' + i) + ":")
			}

			bitMap >>= 1
		}
	}

	return drives, make([]string, 0)
}

func openBrowser(url string) {
	var cmd = exec.Command("cmd", "/c", "start", url)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	cmd.Run()
}

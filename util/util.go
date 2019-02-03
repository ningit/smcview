// Package util provides some utility functions
package util

import (
	"html"
	"strings"
)

// CleanString removes ANSI control codes from a string.
func CleanString(str string) string {
	var length = len(str)
	var builder strings.Builder

	for i := 0; i < length; i++ {
		switch str[i] {
		case '\x1b':
			for i < length && str[i] != 'm' {
				i++
			}
		default:
			builder.WriteByte(str[i])
		}
	}

	return builder.String()
}

// CleanHtmlString does the same as CleanString but also escapes the string
// to be included in HTML code.
func CleanHtmlString(str string) string {
	return html.EscapeString(CleanString(str))
}

// CleanEscapeString cleans a string like CleanHtmlString but also escapes double quotes.
func CleanEscapeString(str string) string {
	return strings.Replace(CleanHtmlString(str), "\"", "\\\"", -1)
}

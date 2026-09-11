// Package htmlclass provides a small helper for building HTML class
// attributes across templ pages, so pages don't need to depend on each other
// (or on web/root specifically) just to share it.
package htmlclass

import "strings"

// Join concatenates class strings with a space, purely for readability when
// a class attribute is built from several named pieces across multiple
// lines. It does not merge or dedupe conflicting Tailwind utilities -- use
// github.com/Oudwins/tailwind-merge-go for that.
func Join(classStrings ...string) string {
	return strings.Join(classStrings, " ")
}

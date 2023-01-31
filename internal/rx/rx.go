// rx is a package containing regexps for Fedichan.
package rx

import (
	"regexp"
)

var Cite = regexp.MustCompile(`(>>(https?://[A-Za-z0-9_.:\-~]+\/[A-Za-z0-9_.\-~]+\/)(f[A-Za-z0-9_.\-~]+-)?([A-Za-z0-9_.\-~]+)?#?([A-Za-z0-9_.\-~]+)?)`)
var CiteEsc = regexp.MustCompile(`(&gt;&gt;(https?://[A-Za-z0-9_.:\-~]+\/[A-Za-z0-9_.\-~]+\/)(f[A-Za-z0-9_.\-~]+-)?([A-Za-z0-9_.\-~]+)?#?([A-Za-z0-9_.\-~]+)?)`)
var LinkTitle = regexp.MustCompile(`(&gt;&gt;(https?://[A-Za-z0-9_.:\-~]+\/[A-Za-z0-9_.\-~]+\/)\w+(#.+)?)`)
var Quote = regexp.MustCompile(`(?m)^\s*&gt;(.+?)$`)
var WordCharsToEnd = regexp.MustCompile(`\w+$`)

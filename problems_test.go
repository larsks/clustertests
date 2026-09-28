package clustertests

import (
	"slices"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gcustom"
)

// expectNoProblems fails the spec unless problems is empty. This is how every
// check that gathers one description per offending object reports them. The
// problems are sorted first so the output is stable however the API server
// happened to order its list.
func expectNoProblems(problems []string) {
	GinkgoHelper()

	Expect(slices.Sorted(slices.Values(problems))).To(haveNoProblems())
}

// haveNoProblems matches an empty []string. Its failure message puts one
// problem on each line, because Gomega's default rendering of a []string is a
// single Go-quoted line that is hard to read in the terminal and in the HTML
// report.
func haveNoProblems() gcustom.CustomGomegaMatcher {
	return gcustom.MakeMatcher(func(problems []string) (bool, error) {
		return len(problems) == 0, nil
	}).WithTemplate(
		"{{if .Failure}}Found {{len .Actual}} problem(s):\n{{range .Actual}}  - {{.}}\n{{end}}" +
			"{{else}}Expected problems, but found none{{end}}",
	)
}

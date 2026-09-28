package main

import (
	"embed"
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"slices"
	"strconv"
	"strings"
)

//go:embed templates/report.html.tmpl
var templateFS embed.FS

// Test is the view model for a single testcase.
type Test struct {
	Name        string
	Classname   string
	Status      string
	TimeSeconds float64
	TimeDisplay string
	Failure     string
	SystemOut   string
	SystemErr   string
	SuiteName   string
}

// Suite is the view model for a single testsuite, rendered as one tab.
type Suite struct {
	Name       string
	Properties []JUnitProperty
	// ServerURL is the value of the "ServerURL" property, if any testcase or
	// the suite itself defines one. On the overview it is every distinct
	// value across suites, comma separated.
	ServerURL string
	Tests     []Test
	Total     int
	Passed    int
	Skipped   int
	Failed    int
}

// Report is the top level view model passed to the template.
type Report struct {
	Suites   []Suite
	Overview Suite
}

func testStatus(tc JUnitTestCase) string {
	switch tc.Status {
	case "passed", "failed", "skipped":
		return tc.Status
	}

	switch {
	case tc.Failure != nil || tc.Error != nil:
		return "failed"
	case tc.Skipped != nil:
		return "skipped"
	default:
		return "passed"
	}
}

func failureText(tc JUnitTestCase) string {
	var msg *JUnitMessage
	switch {
	case tc.Failure != nil:
		msg = tc.Failure
	case tc.Error != nil:
		msg = tc.Error
	default:
		return ""
	}

	text := strings.TrimSpace(msg.Text)
	if text != "" {
		return text
	}
	return msg.Message
}

// serverURLProperty is the name of the property (suite level or testcase
// level) that identifies the cluster a report was generated against.
const serverURLProperty = "ServerURL"

// propertyValue returns the value of the named property. Some producers, such
// as Ginkgo's report entries, JSON-encode values, so a value that is a JSON
// string is unquoted; anything else is returned as is.
func propertyValue(props []JUnitProperty, name string) string {
	for _, p := range props {
		if p.Name != name {
			continue
		}
		var s string
		if err := json.Unmarshal([]byte(p.Value), &s); err == nil {
			return s
		}
		return p.Value
	}
	return ""
}

// suiteServerURL returns the first ServerURL found on the suite's own
// properties or, failing that, on any of its testcases.
func suiteServerURL(s JUnitTestSuite) string {
	if v := propertyValue(s.Properties, serverURLProperty); v != "" {
		return v
	}
	for _, tc := range s.TestCases {
		if v := propertyValue(tc.Properties, serverURLProperty); v != "" {
			return v
		}
	}
	return ""
}

func buildReport(suites []JUnitTestSuite) Report {
	var report Report
	var serverURLs []string

	for _, s := range suites {
		suite := Suite{
			Name:       s.Name,
			Properties: s.Properties,
			ServerURL:  suiteServerURL(s),
		}
		if suite.ServerURL != "" && !slices.Contains(serverURLs, suite.ServerURL) {
			serverURLs = append(serverURLs, suite.ServerURL)
		}

		for _, tc := range s.TestCases {
			status := testStatus(tc)
			seconds, _ := strconv.ParseFloat(tc.Time, 64)

			suite.Tests = append(suite.Tests, Test{
				Name:        tc.Name,
				Classname:   tc.Classname,
				Status:      status,
				TimeSeconds: seconds,
				TimeDisplay: fmt.Sprintf("%.3fs", seconds),
				Failure:     failureText(tc),
				SystemOut:   strings.TrimSpace(tc.SystemOut),
				SystemErr:   strings.TrimSpace(tc.SystemErr),
				SuiteName:   s.Name,
			})

			suite.Total++
			switch status {
			case "passed":
				suite.Passed++
			case "skipped":
				suite.Skipped++
			case "failed":
				suite.Failed++
			}
		}

		report.Suites = append(report.Suites, suite)

		report.Overview.Tests = append(report.Overview.Tests, suite.Tests...)
		report.Overview.Total += suite.Total
		report.Overview.Passed += suite.Passed
		report.Overview.Skipped += suite.Skipped
		report.Overview.Failed += suite.Failed
	}

	report.Overview.ServerURL = strings.Join(serverURLs, ", ")

	return report
}

// Render writes a self-contained HTML report for the given test suites to w.
func Render(w io.Writer, suites []JUnitTestSuite) error {
	tmpl, err := template.ParseFS(templateFS, "templates/report.html.tmpl")
	if err != nil {
		return fmt.Errorf("parsing template: %w", err)
	}

	return tmpl.Execute(w, buildReport(suites))
}

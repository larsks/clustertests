package main

import (
	"embed"
	"fmt"
	"html/template"
	"io"
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
	Tests      []Test
	Total      int
	Passed     int
	Skipped    int
	Failed     int
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

func buildReport(suites []JUnitTestSuite) Report {
	var report Report

	for _, s := range suites {
		suite := Suite{
			Name:       s.Name,
			Properties: s.Properties,
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

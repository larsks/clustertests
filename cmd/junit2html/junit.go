package main

import (
	"bytes"
	"encoding/xml"
	"fmt"
)

// JUnitTestSuites is the root element used when a report wraps one or more
// testsuite elements.
type JUnitTestSuites struct {
	XMLName xml.Name         `xml:"testsuites"`
	Suites  []JUnitTestSuite `xml:"testsuite"`
}

type JUnitTestSuite struct {
	XMLName    xml.Name        `xml:"testsuite"`
	Name       string          `xml:"name,attr"`
	Properties []JUnitProperty `xml:"properties>property"`
	TestCases  []JUnitTestCase `xml:"testcase"`
}

type JUnitProperty struct {
	Name  string `xml:"name,attr"`
	Value string `xml:"value,attr"`
}

type JUnitTestCase struct {
	Name      string        `xml:"name,attr"`
	Classname string        `xml:"classname,attr"`
	Status    string        `xml:"status,attr"`
	Time      string        `xml:"time,attr"`
	Failure   *JUnitMessage `xml:"failure"`
	Error     *JUnitMessage `xml:"error"`
	Skipped   *JUnitMessage `xml:"skipped"`
	SystemOut string        `xml:"system-out"`
	SystemErr string        `xml:"system-err"`
	// Properties holds per-testcase properties, such as Ginkgo report entries.
	Properties []JUnitProperty `xml:"properties>property"`
}

type JUnitMessage struct {
	Message string `xml:"message,attr"`
	Text    string `xml:",chardata"`
}

// parseReport parses a JUnit XML report, accepting either a <testsuites>
// root or a single bare <testsuite> root, and returns the contained test
// suites.
func parseReport(data []byte) ([]JUnitTestSuite, error) {
	dec := xml.NewDecoder(bytes.NewReader(data))
	for {
		tok, err := dec.Token()
		if err != nil {
			return nil, fmt.Errorf("finding root element: %w", err)
		}
		se, ok := tok.(xml.StartElement)
		if !ok {
			continue
		}

		switch se.Name.Local {
		case "testsuites":
			var v JUnitTestSuites
			if err := xml.Unmarshal(data, &v); err != nil {
				return nil, err
			}
			return v.Suites, nil
		case "testsuite":
			var v JUnitTestSuite
			if err := xml.Unmarshal(data, &v); err != nil {
				return nil, err
			}
			return []JUnitTestSuite{v}, nil
		default:
			return nil, fmt.Errorf("unrecognized root element %q", se.Name.Local)
		}
	}
}

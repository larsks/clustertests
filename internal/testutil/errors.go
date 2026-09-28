package testutil

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gcustom"
)

// ExpectNoError fails the spec if err is not nil. It is a drop-in replacement
// for Expect(err).NotTo(HaveOccurred(), description...) that reports only the
// error's message. HaveOccurred prints the message and then dumps the error
// value's fields, which for a Kubernetes StatusError is some thirty lines of
// mostly empty metadata.
func ExpectNoError(err error, description ...any) {
	GinkgoHelper()

	Expect(err).To(gcustom.MakeMatcher(
		func(err error) (bool, error) {
			return err == nil, nil
		},
	).WithTemplate(
		"{{if .Failure}}Unexpected error: {{.Actual}}{{else}}Expected an error, but there was none{{end}}",
	), description...)
}

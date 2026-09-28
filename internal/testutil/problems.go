package testutil

import (
	"slices"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gcustom"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ExpectNoProblems fails the spec unless problems is empty. This is how every
// check that gathers one description per offending object reports them. The
// problems are sorted first so the output is stable however the API server
// happened to order its list.
func ExpectNoProblems(problems []string) {
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

// ObjectID returns namespace/name for namespaced resources and just the name
// for cluster-scoped ones, so that same-named resources in different
// namespaces can be told apart in failure messages. Any *appsv1.Deployment,
// *appsv1.StatefulSet, *appsv1.DaemonSet, or *unstructured.Unstructured
// satisfies metav1.Object, whether through an embedded ObjectMeta or its own
// accessor methods.
func ObjectID(obj metav1.Object) string {
	if namespace := obj.GetNamespace(); namespace != "" {
		return namespace + "/" + obj.GetName()
	}
	return obj.GetName()
}

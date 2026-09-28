package testutil

import (
	"context"
	"fmt"
	"slices"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	apimeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// ConditionReady is the name of the standard readiness condition used by
// Certificates, SecretStores, ExternalSecrets, and most other CRDs.
const ConditionReady = "Ready"

// ConditionsOf decodes status.conditions from any resource into
// metav1.Condition. Projects define their own condition types, but they all
// serialize to the same JSON shape, so this gives a single type for checking
// conditions regardless of which project owns the resource.
func ConditionsOf(obj *unstructured.Unstructured) ([]metav1.Condition, error) {
	raw, _, err := unstructured.NestedSlice(obj.Object, "status", "conditions")
	if err != nil {
		return nil, err
	}

	conditions := make([]metav1.Condition, len(raw))
	for i, item := range raw {
		fields, ok := item.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("status.conditions[%d] is %T, not an object", i, item)
		}
		if err := runtime.DefaultUnstructuredConverter.FromUnstructured(fields, &conditions[i]); err != nil {
			return nil, fmt.Errorf("status.conditions[%d]: %w", i, err)
		}
	}
	return conditions, nil
}

// ConditionExpectation names a condition type and the status it must have.
type ConditionExpectation struct {
	Type   string
	Status metav1.ConditionStatus
}

// ConditionProblems lists every resource of the given type across all
// namespaces and returns a sorted description of each that lacks an expected
// condition with the expected status, along with the number of resources
// checked. A missing condition counts as a problem. Offenders are reported as
// namespace/name along with the condition's reason and message. A condition
// whose observedGeneration is older than the resource's generation is treated
// as stale.
func ConditionProblems(
	ctx context.Context, gvr schema.GroupVersionResource, expected ...ConditionExpectation,
) ([]string, int, error) {
	var problems []string
	count := 0
	err := EachResource(ctx, gvr, metav1.ListOptions{}, func(obj *unstructured.Unstructured) error {
		count++
		id := ObjectID(obj)

		conditions, err := ConditionsOf(obj)
		if err != nil {
			problems = append(problems, fmt.Sprintf("%s: %v", id, err))
			return nil
		}

		for _, want := range expected {
			condition := apimeta.FindStatusCondition(conditions, want.Type)
			switch {
			case condition == nil:
				problems = append(problems, fmt.Sprintf("%s: no %s condition", id, want.Type))
			case condition.Status != want.Status:
				problems = append(problems, fmt.Sprintf(
					"%s: %s=%s, want %s (%s: %s)",
					id, want.Type, condition.Status, want.Status, condition.Reason, condition.Message,
				))
			case condition.ObservedGeneration != 0 && condition.ObservedGeneration < obj.GetGeneration():
				problems = append(problems, fmt.Sprintf(
					"%s: %s is stale (observed generation %d, current generation %d)",
					id, want.Type, condition.ObservedGeneration, obj.GetGeneration(),
				))
			}
		}
		return nil
	})

	slices.Sort(problems)
	return problems, count, err
}

// Presence says whether a check that spans every resource of some type also
// needs at least one of them to exist.
type Presence int

const (
	// NoneOK is for resources that may legitimately not exist, such as ones
	// users create on top of an operator, or a management cluster's hosted
	// clusters before any have been created. An empty list passes.
	NoneOK Presence = iota

	// AtLeastOne is for resources the install itself creates. An empty list
	// fails, since a check that finds nothing to look at would otherwise pass
	// without checking anything.
	AtLeastOne
)

// Expect fails a spec that found count resources, described by what (for
// example "clusteroperators.config.openshift.io"), if p requires at least one
// and none exist.
func (p Presence) Expect(what string, count int) {
	GinkgoHelper()

	if p == AtLeastOne {
		Expect(count).NotTo(BeZero(), "no %s found", what)
	}
}

// ExpectConditions fails unless every resource of the given type, across all
// namespaces, has every expected condition with the expected status (see
// ConditionProblems), and unless at least one exists if p is AtLeastOne.
func ExpectConditions(
	ctx context.Context, gvr schema.GroupVersionResource, p Presence, expected ...ConditionExpectation,
) {
	GinkgoHelper()

	problems, count, err := ConditionProblems(ctx, gvr, expected...)
	ExpectNoError(err, "list %s", gvr.Resource)
	ExpectNoProblems(problems)
	p.Expect(gvr.GroupResource().String(), count)
}

// ExpectAllReady fails unless every resource of the given type has a condition
// of type condType with status True (and, as for ExpectConditions, unless at
// least one exists if p is AtLeastOne).
func ExpectAllReady(ctx context.Context, gvr schema.GroupVersionResource, p Presence, condType string) {
	GinkgoHelper()
	ExpectConditions(ctx, gvr, p, ConditionExpectation{Type: condType, Status: metav1.ConditionTrue})
}

// ExpectPhase lists every object of gvr matching opts and fails unless each has
// status.phase == wantPhase, and unless at least one exists if p is AtLeastOne.
// A missing phase counts as a problem.
func ExpectPhase(
	ctx context.Context, gvr schema.GroupVersionResource, opts metav1.ListOptions, p Presence, wantPhase string,
) {
	GinkgoHelper()

	var problems []string
	count := 0
	err := EachResource(ctx, gvr, opts, func(item *unstructured.Unstructured) error {
		count++
		id := ObjectID(item)

		phase, found, err := unstructured.NestedString(item.Object, "status", "phase")
		if err != nil {
			return fmt.Errorf("read phase for %s: %w", id, err)
		}
		if !found {
			phase = "<missing>"
		}
		if phase != wantPhase {
			problems = append(problems, fmt.Sprintf("%s: phase=%s", id, phase))
		}
		return nil
	})
	ExpectNoError(err, "list %s", gvr.Resource)

	ExpectNoProblems(problems)

	what := gvr.GroupResource().String()
	if opts.LabelSelector != "" {
		what += fmt.Sprintf(" matching label selector %q", opts.LabelSelector)
	}
	p.Expect(what, count)
}

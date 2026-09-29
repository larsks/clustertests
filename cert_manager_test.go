package clustertests

import (
	"context"
	"fmt"
	"slices"
	"time"

	"github.com/larsks/clustertests/internal/testutil"
	. "github.com/onsi/ginkgo/v2"

	apimeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

const (
	certManagerNamespace = "cert-manager"

	// certRenewalGracePeriod is how long after a certificate's status.renewalTime
	// cert-manager is given to finish renewing it before we call the renewal
	// overdue. Renewal normally completes within minutes of renewalTime.
	certRenewalGracePeriod = time.Hour
)

var (
	certificateGVR = schema.GroupVersionResource{
		Group:    "cert-manager.io",
		Version:  "v1",
		Resource: "certificates",
	}
	certificateRequestGVR = schema.GroupVersionResource{
		Group:    "cert-manager.io",
		Version:  "v1",
		Resource: "certificaterequests",
	}
	orderGVR = schema.GroupVersionResource{
		Group:    "acme.cert-manager.io",
		Version:  "v1",
		Resource: "orders",
	}
	challengeGVR = schema.GroupVersionResource{
		Group:    "acme.cert-manager.io",
		Version:  "v1",
		Resource: "challenges",
	}
	issuerGVR = schema.GroupVersionResource{
		Group:    "cert-manager.io",
		Version:  "v1",
		Resource: "issuers",
	}
	clusterIssuerGVR = schema.GroupVersionResource{
		Group:    "cert-manager.io",
		Version:  "v1",
		Resource: "clusterissuers",
	}
)

var _ = Describe("Cert-Manager", Label("cert-manager"), func() {
	BeforeEach(func(ctx SpecContext) {
		testutil.SkipIfResourceKindDoesNotExist(certificateGVR)
	})

	testutil.DescribeAvailableDeployments(certManagerNamespace,
		"cert-manager",
		"cert-manager-cainjector",
		"cert-manager-webhook",
	)

	// An Issuer that isn't Ready (bad credentials, an unreachable ACME
	// server, a missing CA secret) is the root cause behind every certificate
	// that depends on it failing to issue or renew, so report it directly.
	It("has healthy ClusterIssuers", func(ctx SpecContext) {
		testutil.ExpectAllReady(ctx, clusterIssuerGVR, testutil.NoneOK, testutil.ConditionReady)
	})

	It("has healthy Issuers", func(ctx SpecContext) {
		testutil.ExpectAllReady(ctx, issuerGVR, testutil.NoneOK, testutil.ConditionReady)
	})

	It("has healthy certificates", func(ctx SpecContext) {
		testutil.ExpectAllReady(ctx, certificateGVR, testutil.NoneOK, testutil.ConditionReady)
	})

	// A CertificateRequest is what a Certificate hands to an issuer, so when
	// issuance fails, the reason (a rejected ACME order, a denied approval, an
	// invalid CSR) is recorded here rather than on the Certificate. Failed
	// requests stay around as history, though, so one is only reported if its
	// Certificate hasn't since settled (see settledCertificates).
	It("has no failed certificate requests", func(ctx SpecContext) {
		settled, err := settledCertificates(ctx)
		testutil.ExpectNoError(err, "list certificates")

		var problems []string
		err = testutil.EachResource(ctx, certificateRequestGVR, metav1.ListOptions{}, func(request *unstructured.Unstructured) error {
			if certificateRequestIsStale(request, settled) {
				return nil
			}
			id := testutil.ObjectID(request)

			conditions, err := testutil.ConditionsOf(request)
			if err != nil {
				problems = append(problems, fmt.Sprintf("%s: %v", id, err))
				return nil
			}

			ready := apimeta.FindStatusCondition(conditions, testutil.ConditionReady)
			for _, failure := range []string{"Denied", "InvalidRequest"} {
				if condition := apimeta.FindStatusCondition(conditions, failure); condition != nil &&
					condition.Status == metav1.ConditionTrue {
					problems = append(problems, fmt.Sprintf("%s: %s (%s)", id, failure, condition.Message))
					return nil
				}
			}
			if ready != nil && ready.Status == metav1.ConditionFalse && ready.Reason == "Failed" {
				problems = append(problems, fmt.Sprintf("%s: failed (%s)", id, ready.Message))
			}
			return nil
		})
		testutil.ExpectNoError(err, "list certificate requests")

		testutil.ExpectNoProblems(problems)
	})

	// An ACME Order tracks one attempt to get a certificate from an ACME
	// server such as Let's Encrypt. Orders that end in one of these states
	// won't recover on their own, and status.reason carries the ACME
	// server's explanation. As with CertificateRequests, failed Orders
	// remain as history, so ones belonging to a settled Certificate are
	// ignored.
	It("has no failed ACME orders", func(ctx SpecContext) {
		testutil.SkipIfResourceKindDoesNotExist(orderGVR)

		settled, err := settledCertificates(ctx)
		testutil.ExpectNoError(err, "list certificates")
		staleRequests, err := staleCertificateRequests(ctx, settled)
		testutil.ExpectNoError(err, "list certificate requests")

		var problems []string
		err = testutil.EachResource(ctx, orderGVR, metav1.ListOptions{}, func(order *unstructured.Unstructured) error {
			state, _, err := unstructured.NestedString(order.Object, "status", "state")
			if err != nil {
				problems = append(problems, fmt.Sprintf("%s: %v", testutil.ObjectID(order), err))
				return nil
			}
			if !slices.Contains([]string{"invalid", "expired", "errored"}, state) ||
				ownedByStale(order, "CertificateRequest", staleRequests) {
				return nil
			}

			reason, _, _ := unstructured.NestedString(order.Object, "status", "reason")
			problems = append(problems, fmt.Sprintf("%s: state=%s (%s)", testutil.ObjectID(order), state, reason))
			return nil
		})
		testutil.ExpectNoError(err, "list ACME orders")

		testutil.ExpectNoProblems(problems)
	})

	// An ACME Challenge is the proof of domain control that the ACME server
	// asks for, via an HTTP or DNS record. A challenge that has ended in a
	// failed state won't recover, and one that stays pending is stuck, most
	// often because the record can't be reached or created, in which case
	// status.reason holds the failing self-check. Challenges are only
	// expected to be pending for as long as it takes the record to propagate,
	// so that is allowed for before one is reported. As with Orders, ones
	// belonging to a settled Certificate are ignored.
	It("has no failed or stuck ACME challenges", func(ctx SpecContext) {
		testutil.SkipIfResourceKindDoesNotExist(challengeGVR)

		timeout := testutil.GetEnvWithDefault("CHALLENGE_PENDING_TIMEOUT", testutil.DefaultChallengePendingTimeout)
		now := time.Now()

		settled, err := settledCertificates(ctx)
		testutil.ExpectNoError(err, "list certificates")
		staleRequests, err := staleCertificateRequests(ctx, settled)
		testutil.ExpectNoError(err, "list certificate requests")
		staleOrderSet, err := staleOrders(ctx, staleRequests)
		testutil.ExpectNoError(err, "list ACME orders")

		var problems []string
		err = testutil.EachResource(ctx, challengeGVR, metav1.ListOptions{}, func(challenge *unstructured.Unstructured) error {
			id := testutil.ObjectID(challenge)

			state, _, err := unstructured.NestedString(challenge.Object, "status", "state")
			if err != nil {
				problems = append(problems, fmt.Sprintf("%s: %v", id, err))
				return nil
			}
			if ownedByStale(challenge, "Order", staleOrderSet) {
				return nil
			}

			reason, _, _ := unstructured.NestedString(challenge.Object, "status", "reason")
			switch state {
			case "invalid", "expired", "errored":
				problems = append(problems, fmt.Sprintf("%s: state=%s (%s)", id, state, reason))
			case "", "pending":
				if age := now.Sub(challenge.GetCreationTimestamp().Time); age > timeout {
					problems = append(problems, fmt.Sprintf(
						"%s: still pending after %s (%s)", id, age.Round(time.Second), reason,
					))
				}
			}
			return nil
		})
		testutil.ExpectNoError(err, "list ACME challenges")

		testutil.ExpectNoProblems(problems)
	})

	// A Ready=True condition can lag reality, so check the certificate's
	// own dates as well.
	It("has no expired certificates or overdue renewals", func(ctx SpecContext) {
		now := time.Now()
		var problems []string
		err := testutil.EachResource(ctx, certificateGVR, metav1.ListOptions{}, func(certificate *unstructured.Unstructured) error {
			id := testutil.ObjectID(certificate)

			notAfter, hasNotAfter, err := statusTime(certificate, "notAfter")
			if err != nil {
				problems = append(problems, fmt.Sprintf("%s: %v", id, err))
				return nil
			}
			renewalTime, hasRenewalTime, err := statusTime(certificate, "renewalTime")
			if err != nil {
				problems = append(problems, fmt.Sprintf("%s: %v", id, err))
				return nil
			}

			switch {
			case hasNotAfter && now.After(notAfter):
				problems = append(problems, fmt.Sprintf("%s: expired at %s", id, notAfter.Format(time.RFC3339)))
			case hasRenewalTime && now.After(renewalTime.Add(certRenewalGracePeriod)):
				problems = append(problems, fmt.Sprintf(
					"%s: renewal overdue since %s", id, renewalTime.Format(time.RFC3339),
				))
			}
			return nil
		})
		testutil.ExpectNoError(err, "list certificates")

		testutil.ExpectNoProblems(problems)
	})
})

// certificateNameAnnotation is set by cert-manager on each CertificateRequest
// it creates on behalf of a Certificate, naming that Certificate.
const certificateNameAnnotation = "cert-manager.io/certificate-name"

// settledCertificates returns the namespace/name of every Certificate that is
// Ready and not in the middle of issuing or renewing. A Certificate stays
// Ready=True while it renews, as long as its current certificate is valid, so
// Issuing is what tells a renewal that is failing (Issuing=True, still
// retrying) from one that has finished and left only history behind.
func settledCertificates(ctx context.Context) (map[string]bool, error) {
	settled := map[string]bool{}
	err := testutil.EachResource(ctx, certificateGVR, metav1.ListOptions{}, func(certificate *unstructured.Unstructured) error {
		conditions, err := testutil.ConditionsOf(certificate)
		if err != nil {
			// Leave it unsettled; the Ready condition check reports it.
			return nil
		}

		ready := apimeta.IsStatusConditionTrue(conditions, testutil.ConditionReady)
		issuing := apimeta.IsStatusConditionTrue(conditions, "Issuing")
		if ready && !issuing {
			settled[testutil.ObjectID(certificate)] = true
		}
		return nil
	})
	return settled, err
}

// certificateRequestIsStale reports whether request belongs to a Certificate
// that has settled, which makes it leftover history (for example a failed
// attempt that a later one replaced) and not a current problem. A request
// with no owning Certificate, such as one created directly by another
// controller, is never stale.
func certificateRequestIsStale(request *unstructured.Unstructured, settled map[string]bool) bool {
	name, found := request.GetAnnotations()[certificateNameAnnotation]
	return found && settled[request.GetNamespace()+"/"+name]
}

// staleCertificateRequests returns the namespace/name of every
// CertificateRequest that is stale (see certificateRequestIsStale) given the
// set of settled Certificates.
func staleCertificateRequests(ctx context.Context, settled map[string]bool) (map[string]bool, error) {
	stale := map[string]bool{}
	err := testutil.EachResource(ctx, certificateRequestGVR, metav1.ListOptions{}, func(request *unstructured.Unstructured) error {
		if certificateRequestIsStale(request, settled) {
			stale[testutil.ObjectID(request)] = true
		}
		return nil
	})
	return stale, err
}

// ownedByStale reports whether obj has an owner reference to an object of the
// given kind that is in stale. cert-manager creates each ACME Order with an
// owner reference to the CertificateRequest it serves, and each Challenge with
// one to its Order.
func ownedByStale(obj *unstructured.Unstructured, ownerKind string, stale map[string]bool) bool {
	for _, owner := range obj.GetOwnerReferences() {
		if owner.Kind == ownerKind && stale[obj.GetNamespace()+"/"+owner.Name] {
			return true
		}
	}
	return false
}

// staleOrders returns the namespace/name of every ACME Order owned by a stale
// CertificateRequest.
func staleOrders(ctx context.Context, staleRequests map[string]bool) (map[string]bool, error) {
	stale := map[string]bool{}
	err := testutil.EachResource(ctx, orderGVR, metav1.ListOptions{}, func(order *unstructured.Unstructured) error {
		if ownedByStale(order, "CertificateRequest", staleRequests) {
			stale[testutil.ObjectID(order)] = true
		}
		return nil
	})
	return stale, err
}

// statusTime reads an RFC 3339 timestamp from the named field of an object's
// status. The bool result is false if the field is absent, as it is for a
// certificate that has not been issued yet; the Ready condition check reports
// those.
func statusTime(obj *unstructured.Unstructured, field string) (time.Time, bool, error) {
	value, found, err := unstructured.NestedString(obj.Object, "status", field)
	if err != nil || !found {
		return time.Time{}, false, err
	}
	parsed, err := time.Parse(time.RFC3339, value)
	if err != nil {
		return time.Time{}, false, fmt.Errorf("parse status.%s: %w", field, err)
	}
	return parsed, true, nil
}

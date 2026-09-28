package clustertests

import (
	"fmt"
	"slices"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

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

var _ = Describe("CertManager", Label("cert-manager"), func() {
	BeforeEach(func(ctx SpecContext) {
		skipIfResourceKindDoesNotExist(certificateGVR)
	})

	DescribeTable("has available deployment", func(ctx SpecContext, name string) {
		deploymentIsAvailableByName(ctx, certManagerNamespace, name)
	},
		Entry("cert-manager", "cert-manager"),
		Entry("cert-manager-cainjector", "cert-manager-cainjector"),
		Entry("cert-manager-webhook", "cert-manager-webhook"),
	)

	// An Issuer that isn't Ready (bad credentials, an unreachable ACME
	// server, a missing CA secret) is the root cause behind every certificate
	// that depends on it failing to issue or renew, so report it directly.
	It("has healthy ClusterIssuers", func(ctx SpecContext) {
		expectAllReady(ctx, clusterIssuerGVR, conditionReady)
	})

	It("has healthy Issuers", func(ctx SpecContext) {
		expectAllReady(ctx, issuerGVR, conditionReady)
	})

	It("has healthy certificates", func(ctx SpecContext) {
		expectAllReady(ctx, certificateGVR, conditionReady)
	})

	// A Ready=True condition can lag reality, so check the certificate's
	// own dates as well.
	It("has no expired certificates or overdue renewals", func(ctx SpecContext) {
		now := time.Now()
		var problems []string
		err := eachResource(ctx, certificateGVR, metav1.ListOptions{}, func(certificate *unstructured.Unstructured) error {
			id := objectID(certificate)

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
		Expect(err).NotTo(HaveOccurred(), "list certificates")

		slices.Sort(problems)
		Expect(problems).To(BeEmpty())
	})
})

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

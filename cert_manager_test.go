package cluster_tests

import (
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"

	cmapi "github.com/cert-manager/cert-manager/pkg/apis/certmanager/v1"
	cmmeta "github.com/cert-manager/cert-manager/pkg/apis/meta/v1"
)

const (
	certManagerNamespace = "cert-manager"
)

var (
	certificateGVR = schema.GroupVersionResource{
		Group:    "cert-manager.io",
		Version:  "v1",
		Resource: "certificates",
	}
)

var _ = Describe("CertManager", Label("cert-manager"), func() {
	BeforeEach(func(ctx SpecContext) {
		skipIfNamespaceDoesNotExist(ctx, certManagerNamespace)
	})

	DescribeTable("has available deployment", func(ctx SpecContext, name string) {
		deploymentIsAvailableByName(ctx, certManagerNamespace, name)
	},
		Entry("cert-manager", "cert-manager"),
		Entry("cert-manager-cainjector", "cert-manager-cainjector"),
		Entry("cert-manager-webhook", "cert-manager-webhook"),
	)

	It("has healthy certificates", func(ctx SpecContext) {
		certificates, err := dynamicClient.Resource(certificateGVR).
			Namespace(metav1.NamespaceAll).
			List(ctx, metav1.ListOptions{})
		Expect(err).NotTo(HaveOccurred(), "list certificates")

		var problems []string
		for _, item := range certificates.Items {
			statusFields, _, err := unstructured.NestedMap(item.Object, "status")
			var status cmapi.CertificateStatus
			if err == nil {
				err = runtime.DefaultUnstructuredConverter.FromUnstructured(statusFields, &status)
			}
			if err != nil {
				problems = append(problems, item.GetName())
				continue
			}

			ready := hasCondition(
				status.Conditions,
				cmapi.CertificateConditionReady,
				cmmeta.ConditionTrue,
				func(c cmapi.CertificateCondition) (cmapi.CertificateConditionType, cmmeta.ConditionStatus) {
					return c.Type, c.Status
				},
			)
			if !ready {
				problems = append(problems, item.GetName())
			}
		}

		if len(problems) > 0 {
			Fail(fmt.Sprintf("found not ready certificates: %v", problems))
		}
	})
})

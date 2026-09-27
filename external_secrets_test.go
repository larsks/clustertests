package cluster_tests

import (
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"

	esv1 "github.com/external-secrets/external-secrets/apis/externalsecrets/v1"
)

const (
	externalSecretsNamespace = "external-secrets"
)

var (
	clusterSecretStoreGVR = schema.GroupVersionResource{
		Group:    "external-secrets.io",
		Version:  "v1",
		Resource: "clustersecretstores",
	}
	externalSecretGVSR = schema.GroupVersionResource{
		Group:    "external-secrets.io",
		Version:  "v1",
		Resource: "externalsecrets",
	}
)

var _ = Describe("ExternalSecrestsOperator", Label("secrets"), func() {
	DescribeTable("has available deployment", func(ctx SpecContext, name string) {
		deploymentIsAvailableByName(ctx, externalSecretsNamespace, name)
	},
		Entry("external-secrets", "external-secrets"),
		Entry("external-secrets-cert-controller", "external-secrets-cert-controller"),
		Entry("external-secrets-operator-controller-manager", "external-secrets-operator-controller-manager"),
		Entry("external-secrets-webhook", "external-secrets-webhook"),
	)

	It("has healthy ClusterSecretStores", func(ctx SpecContext) {
		stores, err := dynamicClient.Resource(clusterSecretStoreGVR).
			List(ctx, metav1.ListOptions{})
		Expect(err).NotTo(HaveOccurred(), "list ClusterSecretStores")

		var problems []string
		for _, item := range stores.Items {
			var store esv1.ClusterSecretStore
			if err := runtime.DefaultUnstructuredConverter.FromUnstructured(item.Object, &store); err != nil {
				problems = append(problems, item.GetName())
				continue
			}

			ready := hasCondition(
				store.Status.Conditions,
				esv1.SecretStoreReady,
				corev1.ConditionTrue,
				func(c esv1.SecretStoreStatusCondition) (
					esv1.SecretStoreConditionType,
					corev1.ConditionStatus,
				) {
					return c.Type, c.Status
				},
			)

			if !ready {
				problems = append(problems, store.Name)
			}
		}

		if len(problems) > 0 {
			Fail(fmt.Sprintf("found not ready ClusterSecretStores: %v", problems))
		}
	})

	It("has healthy external secrets", func(ctx SpecContext) {
		secrets, err := dynamicClient.Resource(externalSecretGVSR).
			List(ctx, metav1.ListOptions{})
		Expect(err).NotTo(HaveOccurred(), "list external secrets")

		var problems []string
		for _, item := range secrets.Items {
			var secret esv1.ExternalSecret
			if err := runtime.DefaultUnstructuredConverter.FromUnstructured(item.Object, &secret); err != nil {
				problems = append(problems, item.GetName())
				continue
			}
			ready := hasCondition(
				secret.Status.Conditions,
				esv1.ExternalSecretReady,
				corev1.ConditionTrue,
				func(c esv1.ExternalSecretStatusCondition) (
					esv1.ExternalSecretConditionType,
					corev1.ConditionStatus,
				) {
					return c.Type, c.Status
				},
			)
			if !ready {
				problems = append(problems, secret.Name)
			}
		}
		if len(problems) > 0 {
			Fail(fmt.Sprintf("found not ready external secrets: %v", problems))
		}
	})
})

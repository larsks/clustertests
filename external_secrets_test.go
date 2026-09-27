package cluster_tests

import (
	"context"
	"sort"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"

	esv1 "github.com/external-secrets/external-secrets/apis/externalsecrets/v1"
)

const (
	externalSecretsNamespace = "external-secrets"
)

var (
	secretStoreGVR = schema.GroupVersionResource{
		Group:    "external-secrets.io",
		Version:  "v1",
		Resource: "secretstores",
	}
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

func expectHealthySecretStores(
	ctx context.Context,
	kind string,
	list func(context.Context) (*unstructured.UnstructuredList, error),
) {
	GinkgoHelper()

	stores, err := list(ctx)
	Expect(err).NotTo(HaveOccurred(), "list %s", kind)

	var problems []string
	for _, item := range stores.Items {
		statusFields, _, err := unstructured.NestedMap(item.Object, "status")
		var status esv1.SecretStoreStatus
		if err == nil {
			err = runtime.DefaultUnstructuredConverter.FromUnstructured(statusFields, &status)
		}
		if err != nil {
			problems = append(problems, item.GetName())
			continue
		}

		ready := hasCondition(
			status.Conditions,
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
			problems = append(problems, item.GetName())
		}
	}

	sort.Strings(problems)
	Expect(problems).To(BeEmpty(), "%s not ready: %s", kind, strings.Join(problems, ", "))
}

var _ = Describe("ExternalSecretsOperator", Label("secrets"), func() {
	BeforeEach(func(ctx SpecContext) {
		skipIfNamespaceDoesNotExist(ctx, externalSecretsNamespace)
	})

	DescribeTable("has available deployment", func(ctx SpecContext, name string) {
		deploymentIsAvailableByName(ctx, externalSecretsNamespace, name)
	},
		Entry("external-secrets", "external-secrets"),
		Entry("external-secrets-cert-controller", "external-secrets-cert-controller"),
		Entry("external-secrets-operator-controller-manager", "external-secrets-operator-controller-manager"),
		Entry("external-secrets-webhook", "external-secrets-webhook"),
	)

	It("has healthy ClusterSecretStores", func(ctx SpecContext) {
		expectHealthySecretStores(
			ctx,
			"ClusterSecretStores",
			func(ctx context.Context) (*unstructured.UnstructuredList, error) {
				return dynamicClient.Resource(clusterSecretStoreGVR).List(ctx, metav1.ListOptions{})
			},
		)
	})

	It("has healthy SecretStores", func(ctx SpecContext) {
		expectHealthySecretStores(
			ctx,
			"SecretStores",
			func(ctx context.Context) (*unstructured.UnstructuredList, error) {
				return dynamicClient.Resource(secretStoreGVR).
					Namespace(metav1.NamespaceAll).
					List(ctx, metav1.ListOptions{})
			},
		)
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
		sort.Strings(problems)
		Expect(problems).To(BeEmpty(), "ExternalSecrets not ready: %s", strings.Join(problems, ", "))
	})
})

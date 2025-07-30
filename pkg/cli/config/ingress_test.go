package config

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/fake"
)

// TestGetIngresses tests the getIngresses function
func TestGetIngresses(t *testing.T) {
	tests := []struct {
		name           string
		namespace      string
		setupIngresses []networkingv1.Ingress
		expectedCount  int
		description    string
	}{
		{
			name:      "no ingresses in namespace",
			namespace: "tekton-pipelines",
			setupIngresses: []networkingv1.Ingress{
				// Ingress in different namespace
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "other-ingress",
						Namespace: "other-namespace",
					},
					Spec: networkingv1.IngressSpec{
						Rules: []networkingv1.IngressRule{
							{
								Host: "other.local",
								IngressRuleValue: networkingv1.IngressRuleValue{
									HTTP: &networkingv1.HTTPIngressRuleValue{
										Paths: []networkingv1.HTTPIngressPath{
											{
												Backend: networkingv1.IngressBackend{
													Service: &networkingv1.IngressServiceBackend{
														Name: "tekton-results-api-service",
													},
												},
											},
										},
									},
								},
							},
						},
					},
				},
			},
			expectedCount: 0,
			description:   "should return no ingresses when none match in target namespace",
		},
		{
			name:      "tekton results ingress found",
			namespace: "tekton-pipelines",
			setupIngresses: []networkingv1.Ingress{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "tekton-results-ingress",
						Namespace: "tekton-pipelines",
					},
					Spec: networkingv1.IngressSpec{
						Rules: []networkingv1.IngressRule{
							{
								Host: "tekton-results.local",
								IngressRuleValue: networkingv1.IngressRuleValue{
									HTTP: &networkingv1.HTTPIngressRuleValue{
										Paths: []networkingv1.HTTPIngressPath{
											{
												Backend: networkingv1.IngressBackend{
													Service: &networkingv1.IngressServiceBackend{
														Name: "tekton-results-api-service",
													},
												},
											},
										},
									},
								},
							},
						},
					},
				},
			},
			expectedCount: 1,
			description:   "should find tekton results ingress",
		},
		{
			name:      "multiple ingresses with mixed services",
			namespace: "tekton-pipelines",
			setupIngresses: []networkingv1.Ingress{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "tekton-results-ingress",
						Namespace: "tekton-pipelines",
					},
					Spec: networkingv1.IngressSpec{
						Rules: []networkingv1.IngressRule{
							{
								Host: "tekton-results.local",
								IngressRuleValue: networkingv1.IngressRuleValue{
									HTTP: &networkingv1.HTTPIngressRuleValue{
										Paths: []networkingv1.HTTPIngressPath{
											{
												Backend: networkingv1.IngressBackend{
													Service: &networkingv1.IngressServiceBackend{
														Name: "tekton-results-api-service",
													},
												},
											},
										},
									},
								},
							},
						},
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "other-ingress",
						Namespace: "tekton-pipelines",
					},
					Spec: networkingv1.IngressSpec{
						Rules: []networkingv1.IngressRule{
							{
								Host: "other.local",
								IngressRuleValue: networkingv1.IngressRuleValue{
									HTTP: &networkingv1.HTTPIngressRuleValue{
										Paths: []networkingv1.HTTPIngressPath{
											{
												Backend: networkingv1.IngressBackend{
													Service: &networkingv1.IngressServiceBackend{
														Name: "other-service",
													},
												},
											},
										},
									},
								},
							},
						},
					},
				},
			},
			expectedCount: 1,
			description:   "should find only tekton results ingress among multiple ingresses",
		},
		{
			name:      "multiple tekton results ingresses",
			namespace: "tekton-pipelines",
			setupIngresses: []networkingv1.Ingress{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "tekton-results-ingress-1",
						Namespace: "tekton-pipelines",
					},
					Spec: networkingv1.IngressSpec{
						Rules: []networkingv1.IngressRule{
							{
								Host: "tekton-results-1.local",
								IngressRuleValue: networkingv1.IngressRuleValue{
									HTTP: &networkingv1.HTTPIngressRuleValue{
										Paths: []networkingv1.HTTPIngressPath{
											{
												Backend: networkingv1.IngressBackend{
													Service: &networkingv1.IngressServiceBackend{
														Name: "tekton-results-api-service",
													},
												},
											},
										},
									},
								},
							},
						},
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "tekton-results-ingress-2",
						Namespace: "tekton-pipelines",
					},
					Spec: networkingv1.IngressSpec{
						Rules: []networkingv1.IngressRule{
							{
								Host: "tekton-results-2.local",
								IngressRuleValue: networkingv1.IngressRuleValue{
									HTTP: &networkingv1.HTTPIngressRuleValue{
										Paths: []networkingv1.HTTPIngressPath{
											{
												Backend: networkingv1.IngressBackend{
													Service: &networkingv1.IngressServiceBackend{
														Name: "tekton-results-api-service",
													},
												},
											},
										},
									},
								},
							},
						},
					},
				},
			},
			expectedCount: 2,
			description:   "should find multiple tekton results ingresses",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create fake clientset with test data
			objects := make([]runtime.Object, len(tt.setupIngresses))
			for i, ing := range tt.setupIngresses {
				ingCopy := ing
				objects[i] = &ingCopy
			}

			// Add the target namespace
			objects = append(objects, &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: tt.namespace,
				},
			})

			fakeClientset := fake.NewClientset(objects...)

			// Test the actual getIngressesWithClient function
			ingresses, err := getIngressesWithClient(fakeClientset, tt.namespace)

			// Check for unexpected errors
			if err != nil {
				t.Errorf("%s: unexpected error: %v", tt.description, err)
				return
			}

			// Check the count
			if len(ingresses) != tt.expectedCount {
				t.Errorf("%s: expected %d tekton results ingresses, got %d",
					tt.description, tt.expectedCount, len(ingresses))
			}

			// Verify each found ingress is indeed a tekton results ingress
			for _, ingress := range ingresses {
				if !isTektonResultsIngress(*ingress) {
					t.Errorf("%s: ingress %s/%s should be a tekton results ingress",
						tt.description, ingress.Namespace, ingress.Name)
				}
			}

			// Test URL construction
			if len(ingresses) > 0 {
				urls := constructIngressURLs(ingresses)
				// Basic sanity check - should have at least one URL per ingress
				if len(urls) == 0 {
					t.Errorf("%s: expected URLs to be constructed, got none", tt.description)
				}
			}
		})
	}
}

// TestIsTektonResultsIngress tests the isTektonResultsIngress function
func TestIsTektonResultsIngress(t *testing.T) {
	tests := []struct {
		name        string
		ingress     networkingv1.Ingress
		expected    bool
		description string
	}{
		{
			name: "ingress pointing to tekton-results-api-service",
			ingress: networkingv1.Ingress{
				Spec: networkingv1.IngressSpec{
					Rules: []networkingv1.IngressRule{
						{
							Host: "tekton-results.local",
							IngressRuleValue: networkingv1.IngressRuleValue{
								HTTP: &networkingv1.HTTPIngressRuleValue{
									Paths: []networkingv1.HTTPIngressPath{
										{
											Path: "/",
											PathType: func() *networkingv1.PathType {
												pt := networkingv1.PathTypePrefix
												return &pt
											}(),
											Backend: networkingv1.IngressBackend{
												Service: &networkingv1.IngressServiceBackend{
													Name: "tekton-results-api-service",
													Port: networkingv1.ServiceBackendPort{
														Number: 8080,
													},
												},
											},
										},
									},
								},
							},
						},
					},
				},
			},
			expected:    true,
			description: "should return true for ingress pointing to tekton-results-api-service",
		},
		{
			name: "ingress pointing to other service",
			ingress: networkingv1.Ingress{
				Spec: networkingv1.IngressSpec{
					Rules: []networkingv1.IngressRule{
						{
							Host: "other-service.local",
							IngressRuleValue: networkingv1.IngressRuleValue{
								HTTP: &networkingv1.HTTPIngressRuleValue{
									Paths: []networkingv1.HTTPIngressPath{
										{
											Path: "/",
											PathType: func() *networkingv1.PathType {
												pt := networkingv1.PathTypePrefix
												return &pt
											}(),
											Backend: networkingv1.IngressBackend{
												Service: &networkingv1.IngressServiceBackend{
													Name: "other-service",
													Port: networkingv1.ServiceBackendPort{
														Number: 8080,
													},
												},
											},
										},
									},
								},
							},
						},
					},
				},
			},
			expected:    false,
			description: "should return false for ingress pointing to other service",
		},
		{
			name: "ingress with no rules",
			ingress: networkingv1.Ingress{
				Spec: networkingv1.IngressSpec{
					Rules: []networkingv1.IngressRule{},
				},
			},
			expected:    false,
			description: "should return false for ingress with no rules",
		},
		{
			name: "ingress with no HTTP rules",
			ingress: networkingv1.Ingress{
				Spec: networkingv1.IngressSpec{
					Rules: []networkingv1.IngressRule{
						{
							Host: "tekton-results.local",
						},
					},
				},
			},
			expected:    false,
			description: "should return false for ingress with no HTTP rules",
		},
		{
			name: "ingress with no service backend",
			ingress: networkingv1.Ingress{
				Spec: networkingv1.IngressSpec{
					Rules: []networkingv1.IngressRule{
						{
							Host: "tekton-results.local",
							IngressRuleValue: networkingv1.IngressRuleValue{
								HTTP: &networkingv1.HTTPIngressRuleValue{
									Paths: []networkingv1.HTTPIngressPath{
										{
											Path: "/",
											PathType: func() *networkingv1.PathType {
												pt := networkingv1.PathTypePrefix
												return &pt
											}(),
											Backend: networkingv1.IngressBackend{
												Resource: &corev1.TypedLocalObjectReference{
													Name: "some-resource",
												},
											},
										},
									},
								},
							},
						},
					},
				},
			},
			expected:    false,
			description: "should return false for ingress with resource backend instead of service",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isTektonResultsIngress(tt.ingress)
			if result != tt.expected {
				t.Errorf("%s: isTektonResultsIngress() = %v, want %v", tt.description, result, tt.expected)
			}
		})
	}
}

// TestConstructIngressURLs tests the constructIngressURLs function
func TestConstructIngressURLs(t *testing.T) {
	tests := []struct {
		name         string
		ingresses    []*networkingv1.Ingress
		expectedURLs []string
		description  string
	}{
		{
			name:         "empty ingresses",
			ingresses:    []*networkingv1.Ingress{},
			expectedURLs: []string{},
			description:  "should return empty URLs for empty ingresses",
		},
		{
			name:         "nil ingresses",
			ingresses:    nil,
			expectedURLs: []string{},
			description:  "should return empty URLs for nil ingresses",
		},
		{
			name: "ingress with HTTP only",
			ingresses: []*networkingv1.Ingress{
				{
					Spec: networkingv1.IngressSpec{
						Rules: []networkingv1.IngressRule{
							{
								Host: "tekton-results.local",
							},
						},
					},
				},
			},
			expectedURLs: []string{"http://tekton-results.local"},
			description:  "should return HTTP URL for ingress without TLS",
		},
		{
			name: "ingress with HTTPS (TLS configured)",
			ingresses: []*networkingv1.Ingress{
				{
					Spec: networkingv1.IngressSpec{
						TLS: []networkingv1.IngressTLS{
							{
								Hosts:      []string{"tekton-results.local"},
								SecretName: "tekton-results-tls",
							},
						},
						Rules: []networkingv1.IngressRule{
							{
								Host: "tekton-results.local",
							},
						},
					},
				},
			},
			expectedURLs: []string{"https://tekton-results.local"},
			description:  "should return HTTPS URL for ingress with TLS",
		},
		{
			name: "ingress with multiple hosts",
			ingresses: []*networkingv1.Ingress{
				{
					Spec: networkingv1.IngressSpec{
						Rules: []networkingv1.IngressRule{
							{
								Host: "tekton-results.local",
							},
							{
								Host: "tekton-results.example.com",
							},
						},
					},
				},
			},
			expectedURLs: []string{"http://tekton-results.local", "http://tekton-results.example.com"},
			description:  "should return multiple URLs for ingress with multiple hosts",
		},
		{
			name: "ingress with mixed TLS and non-TLS hosts",
			ingresses: []*networkingv1.Ingress{
				{
					Spec: networkingv1.IngressSpec{
						TLS: []networkingv1.IngressTLS{
							{
								Hosts:      []string{"secure.tekton-results.local"},
								SecretName: "tekton-results-tls",
							},
						},
						Rules: []networkingv1.IngressRule{
							{
								Host: "tekton-results.local",
							},
							{
								Host: "secure.tekton-results.local",
							},
						},
					},
				},
			},
			expectedURLs: []string{"http://tekton-results.local", "https://secure.tekton-results.local"},
			description:  "should return mixed HTTP/HTTPS URLs based on TLS configuration",
		},
		{
			name: "multiple ingresses",
			ingresses: []*networkingv1.Ingress{
				{
					Spec: networkingv1.IngressSpec{
						Rules: []networkingv1.IngressRule{
							{
								Host: "tekton-results-1.local",
							},
						},
					},
				},
				{
					Spec: networkingv1.IngressSpec{
						TLS: []networkingv1.IngressTLS{
							{
								Hosts:      []string{"tekton-results-2.local"},
								SecretName: "tekton-results-tls-2",
							},
						},
						Rules: []networkingv1.IngressRule{
							{
								Host: "tekton-results-2.local",
							},
						},
					},
				},
			},
			expectedURLs: []string{"http://tekton-results-1.local", "https://tekton-results-2.local"},
			description:  "should return URLs for multiple ingresses",
		},
		{
			name: "ingress with empty host",
			ingresses: []*networkingv1.Ingress{
				{
					Spec: networkingv1.IngressSpec{
						Rules: []networkingv1.IngressRule{
							{
								Host: "",
							},
							{
								Host: "tekton-results.local",
							},
						},
					},
				},
			},
			expectedURLs: []string{"http://tekton-results.local"},
			description:  "should skip empty hosts and return only valid URLs",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := constructIngressURLs(tt.ingresses)

			// Compare lengths first
			if len(result) != len(tt.expectedURLs) {
				t.Errorf("%s: constructIngressURLs() returned %d URLs, want %d. Got: %v, Want: %v",
					tt.description, len(result), len(tt.expectedURLs), result, tt.expectedURLs)
				return
			}

			// Compare each URL
			for i, url := range result {
				if i >= len(tt.expectedURLs) || url != tt.expectedURLs[i] {
					t.Errorf("%s: constructIngressURLs()[%d] = %v, want %v", tt.description, i, url, tt.expectedURLs[i])
				}
			}
		})
	}
}

package client

import (
	"context"
	"crypto/ed25519"
	"crypto/x509"
	"encoding/pem"
	"math/big"
	"os"
	"testing"

	"github.com/tektoncd/results/pkg/cli/config"
	v1 "k8s.io/api/authentication/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/fake"
	ktest "k8s.io/client-go/testing"
)

func TestToken(t *testing.T) {
	ctx := context.Background()
	for _, tc := range []struct {
		name    string
		factory *Factory
		want    string
	}{
		{
			name:    "default",
			factory: &Factory{},
			want:    "",
		},
		{
			name: "token",
			factory: &Factory{
				cfg: &config.Config{
					Token: "a",
				},
			},
			want: "a",
		},
		{
			name: "serviceaccount",
			factory: &Factory{
				cfg: &config.Config{
					ServiceAccount: &config.ServiceAccount{
						Namespace: "foo",
						Name:      "bar",
					},
				},
				k8s: func() *fake.Clientset {
					clientset := fake.NewSimpleClientset()
					// Token is a subresource of ServiceAccount, so this
					// operation looks like a SA creation w.r.t. fake clients.
					clientset.PrependReactor("create", "serviceaccounts", func(action ktest.Action) (handled bool, ret runtime.Object, err error) {
						return true, &v1.TokenRequest{
							Status: v1.TokenRequestStatus{
								Token: "a",
							},
						}, nil
					})
					return clientset
				}(),
			},
			want: "a",
		},
		{
			name: "token",
			factory: &Factory{
				cfg: &config.Config{
					Token: "a",
				},
			},
			want: "a",
		},
		{
			name: "token over serviceaccount",
			factory: &Factory{
				cfg: &config.Config{
					Token: "a",
					ServiceAccount: &config.ServiceAccount{
						Namespace: "foo",
						Name:      "bar",
					},
				},
				k8s: fake.NewSimpleClientset(),
			},
			want: "a",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := tc.factory.token(ctx)
			if err != nil {
				t.Fatal(err)
			}
			if got != tc.want {
				t.Errorf("got: %s, want %s", got, tc.want)
			}
		})
	}
}

func TestCerts(t *testing.T) {
	// Generate dummy keypair + cert
	pub, priv, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatalf("ed25519.GenerateKey: %v", err)
	}
	cert, err := x509.CreateCertificate(nil, &x509.Certificate{
		SerialNumber: big.NewInt(0),
	}, &x509.Certificate{}, pub, priv)
	if err != nil {
		t.Fatalf("CreateCertificate: %v", err)
	}

	// Dump cert into file so that the Factory can read it.
	f, err := os.CreateTemp("", "cert*")
	if err != nil {
		t.Fatalf("os.CreateTemp: %v", err)
	}
	defer os.Remove(f.Name())
	if err := pem.Encode(f, &pem.Block{Type: "CERTIFICATE", Bytes: cert}); err != nil {
		t.Fatalf("pem.Encode: %v", err)
	}
	f.Close()

	factory := &Factory{
		cfg: &config.Config{
			SSL: config.SSLConfig{
				RootsFilePath: f.Name(),
			},
		},
	}
	// Reading and parsing out the file is enough to consider this as success.
	if _, err := factory.certs(); err != nil {
		t.Error(err)
	}

}

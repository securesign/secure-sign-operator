package common

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"testing"

	rhtasv1alpha1 "github.com/securesign/operator/api/v1alpha1"
	cm "github.com/securesign/operator/internal/controller/common"
	"github.com/securesign/operator/internal/controller/common/action"
	"github.com/securesign/operator/internal/controller/constants"
	tsaUtils "github.com/securesign/operator/internal/controller/tsa/utils"
	testAction "github.com/securesign/operator/internal/testing/action"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func GenerateTSAInstance() *rhtasv1alpha1.TimestampAuthority {
	return &rhtasv1alpha1.TimestampAuthority{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "timestampAuthority",
			Namespace: "default",
		},
		Status: rhtasv1alpha1.TimestampAuthorityStatus{
			Conditions: []metav1.Condition{
				{
					Type:   constants.Ready,
					Reason: constants.Ready,
				},
			},
			NTPMonitoring: nil,
			Signer:        nil,
		},
		Spec: rhtasv1alpha1.TimestampAuthoritySpec{
			Signer: rhtasv1alpha1.TimestampAuthoritySigner{
				CertificateChain: rhtasv1alpha1.CertificateChain{
					RootCA: rhtasv1alpha1.TsaCertificateAuthority{
						OrganizationName: "Red Hat",
					},
					IntermediateCA: []rhtasv1alpha1.TsaCertificateAuthority{
						{
							OrganizationName: "Red Hat",
						},
					},
					LeafCA: rhtasv1alpha1.TsaCertificateAuthority{
						OrganizationName: "Red Hat",
					},
				},
			},
			NTPMonitoring: rhtasv1alpha1.NTPMonitoring{
				Enabled: true,
				Config: &rhtasv1alpha1.NtpMonitoringConfig{
					RequestAttempts: 3,
					RequestTimeout:  5,
					NumServers:      4,
					ServerThreshold: 3,
					MaxTimeDelta:    6,
					Period:          60,
					Servers:         []string{"time.apple.com", "time.google.com"},
				},
			},
		},
	}
}

func TsaTestSetup(instance *rhtasv1alpha1.TimestampAuthority, t *testing.T, client client.WithWatch, action action.Action[*rhtasv1alpha1.TimestampAuthority], initObjs ...client.Object) (client.WithWatch, action.Action[*rhtasv1alpha1.TimestampAuthority]) {
	if client == nil {
		client = testAction.FakeClientBuilder().WithObjects(instance).Build()
	}
	if err := client.Get(context.TODO(), types.NamespacedName{Name: instance.GetName(), Namespace: instance.GetNamespace()}, instance); err != nil {
		t.Error(err)
		return nil, nil
	}

	for _, obj := range initObjs {
		if err := client.Create(context.TODO(), obj); err != nil {
			t.Error(err)
			return nil, nil
		}
	}

	a := testAction.PrepareAction(client, action)
	if !a.CanHandle(context.TODO(), instance) {
		return nil, nil
	}

	_ = a.Handle(context.TODO(), instance)
	return client, a
}

func HandleTsaTestSecret(instance *rhtasv1alpha1.TimestampAuthority, client client.WithWatch) (*corev1.Secret, error) {
	config := &tsaUtils.TsaCertChainConfig{}

	if instance.Spec.Signer.CertificateChain.CertificateChainRef != nil {
		instance.Spec.Signer.CertificateChain.RootCA = rhtasv1alpha1.TsaCertificateAuthority{
			OrganizationName: "test org",
		}

		instance.Spec.Signer.CertificateChain.IntermediateCA = append(instance.Spec.Signer.CertificateChain.IntermediateCA, rhtasv1alpha1.TsaCertificateAuthority{
			OrganizationName: "test org",
		})

		instance.Spec.Signer.CertificateChain.LeafCA = rhtasv1alpha1.TsaCertificateAuthority{
			OrganizationName: "test org",
		}
	}

	rootKey, err := ecdsa.GenerateKey(elliptic.P384(), rand.Reader)
	if err != nil {
		return nil, err
	}
	rootPassword := cm.GeneratePassword(8)
	config.RootPrivateKeyPassword = rootPassword
	rootCAPrivKey, err := tsaUtils.CreatePrivateKey(rootKey, rootPassword)
	if err != nil {
		return nil, err
	}
	config.RootPrivateKey = rootCAPrivKey

	intermediateKey, err := ecdsa.GenerateKey(elliptic.P384(), rand.Reader)
	if err != nil {
		return nil, err
	}
	intermediatePassword := cm.GeneratePassword(8)
	config.IntermediatePrivateKeyPasswords = append(config.IntermediatePrivateKeyPasswords, intermediatePassword)
	intermediateCAPrivKey, err := tsaUtils.CreatePrivateKey(intermediateKey, intermediatePassword)
	if err != nil {
		return nil, err
	}
	config.IntermediatePrivateKeys = append(config.IntermediatePrivateKeys, intermediateCAPrivKey)

	leafKey, err := ecdsa.GenerateKey(elliptic.P384(), rand.Reader)
	if err != nil {
		return nil, err
	}
	leafPassword := cm.GeneratePassword(8)
	config.LeafPrivateKeyPassword = leafPassword
	leafCAPrivKey, err := tsaUtils.CreatePrivateKey(leafKey, leafPassword)
	if err != nil {
		return nil, err
	}
	config.LeafPrivateKey = leafCAPrivKey

	certificateChain, err := tsaUtils.CreateTSACertChain(context.TODO(), instance, instance.Name, client, config)
	if err != nil {
		return nil, err
	}
	config.CertificateChain = certificateChain

	return &corev1.Secret{
		ObjectMeta: v1.ObjectMeta{
			Name:      "tsa-test-secret",
			Namespace: instance.GetNamespace(),
		},
		Data: map[string][]byte{
			"rootKey":             config.RootPrivateKey,
			"rootKeyPass":         config.RootPrivateKeyPassword,
			"intermediateKey":     config.IntermediatePrivateKeys[0],
			"intermediateKeyPass": config.IntermediatePrivateKeyPasswords[0],
			"leafKey":             config.LeafPrivateKey,
			"leafKeyPass":         config.LeafPrivateKeyPassword,
			"certChain":           config.CertificateChain,
		},
	}, nil
}

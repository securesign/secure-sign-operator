package fips

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"testing"

	. "github.com/onsi/gomega"
	rhtasv1 "github.com/securesign/operator/api/v1"
	"github.com/securesign/operator/internal/constants"
	"github.com/securesign/operator/internal/state"
	testAction "github.com/securesign/operator/internal/testing/action"
	fipsutil "github.com/securesign/operator/internal/utils/fips"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

const (
	testCondition    = "TestSignerCondition"
	testComponent    = "test-component"
	testInstanceName = "test-instance"
	testNamespace    = "default"
)

func testInstance(conditions ...metav1.Condition) *rhtasv1.Rekor {
	instance := &rhtasv1.Rekor{
		ObjectMeta: metav1.ObjectMeta{
			Name: testInstanceName, Namespace: testNamespace, Generation: 1,
		},
	}
	for _, c := range conditions {
		meta.SetStatusCondition(&instance.Status.Conditions, c)
	}
	return instance
}

func pendingConditions() []metav1.Condition {
	return []metav1.Condition{
		{Type: constants.ReadyCondition, Status: metav1.ConditionFalse, Reason: state.Pending.String()},
		{Type: testCondition, Status: metav1.ConditionFalse, Reason: state.Pending.String()},
	}
}

func testFIPSWrapper(
	passwordRef func(*rhtasv1.Rekor) *rhtasv1.SecretKeySelector,
	cryptoMaterial func(context.Context, *rhtasv1.Rekor, client.Client) ([]CryptoRef, error),
) func(*rhtasv1.Rekor) *wrapper[*rhtasv1.Rekor] {
	return Wrapper(Config[*rhtasv1.Rekor]{
		PasswordRef:    passwordRef,
		CryptoMaterial: cryptoMaterial,
		IsEnabled:      func(_ *rhtasv1.Rekor) bool { return true },
	})
}

func generateRSAKeyPEM(t *testing.T, bits int) []byte {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, bits)
	if err != nil {
		t.Fatal(err)
	}
	der := x509.MarshalPKCS1PrivateKey(key)
	var buf bytes.Buffer
	if err := pem.Encode(&buf, &pem.Block{Type: "RSA PRIVATE KEY", Bytes: der}); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func generateEd25519KeyPEM(t *testing.T) []byte {
	t.Helper()
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	der, err := x509.MarshalPKCS8PrivateKey(priv)
	if err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	if err := pem.Encode(&buf, &pem.Block{Type: "PRIVATE KEY", Bytes: der}); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestCanHandle(t *testing.T) {
	original := fipsutil.Enabled
	fipsutil.Enabled = func() bool { return true }
	t.Cleanup(func() { fipsutil.Enabled = original })

	tests := []struct {
		name      string
		instance  *rhtasv1.Rekor
		canHandle bool
	}{
		{
			name:      "no ReadyCondition",
			instance:  testInstance(),
			canHandle: false,
		},
		{
			name: "ReadyCondition below Pending",
			instance: testInstance(metav1.Condition{
				Type: constants.ReadyCondition, Status: metav1.ConditionFalse, Reason: state.NotDefined.String(),
			}),
			canHandle: false,
		},
		{
			name: "no component condition",
			instance: testInstance(metav1.Condition{
				Type: constants.ReadyCondition, Status: metav1.ConditionFalse, Reason: state.Pending.String(),
			}),
			canHandle: true,
		},
		{
			name:      "component condition false",
			instance:  testInstance(pendingConditions()...),
			canHandle: true,
		},
		{
			name: "component condition true, matching generation",
			instance: testInstance(
				metav1.Condition{Type: constants.ReadyCondition, Status: metav1.ConditionFalse, Reason: state.Pending.String()},
				metav1.Condition{Type: testCondition, Status: metav1.ConditionTrue, Reason: "Resolved", ObservedGeneration: 1},
			),
			canHandle: false,
		},
		{
			name: "component condition true, stale generation",
			instance: testInstance(
				metav1.Condition{Type: constants.ReadyCondition, Status: metav1.ConditionFalse, Reason: state.Pending.String()},
				metav1.Condition{Type: testCondition, Status: metav1.ConditionTrue, Reason: "Resolved", ObservedGeneration: 0},
			),
			canHandle: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)
			cli := testAction.FakeClientBuilder().Build()
			a := testAction.PrepareAction(cli, NewAction(
				testCondition, testComponent,
				testFIPSWrapper(nil, nil),
			))
			g.Expect(a.CanHandle(t.Context(), tt.instance)).To(Equal(tt.canHandle))
		})
	}
}

func TestCanHandle_FIPSDisabled(t *testing.T) {
	original := fipsutil.Enabled
	fipsutil.Enabled = func() bool { return false }
	t.Cleanup(func() { fipsutil.Enabled = original })

	t.Run("returns false when FIPS is disabled", func(t *testing.T) {
		g := NewWithT(t)
		instance := testInstance(pendingConditions()...)

		cli := testAction.FakeClientBuilder().Build()
		a := testAction.PrepareAction(cli, NewAction(
			testCondition, testComponent,
			testFIPSWrapper(nil, nil),
		))

		g.Expect(a.CanHandle(t.Context(), instance)).To(BeFalse())
	})
}

func TestCanHandle_IsEnabledFalse(t *testing.T) {
	original := fipsutil.Enabled
	fipsutil.Enabled = func() bool { return true }
	t.Cleanup(func() { fipsutil.Enabled = original })

	t.Run("returns false when IsEnabled returns false", func(t *testing.T) {
		g := NewWithT(t)
		instance := testInstance(pendingConditions()...)

		cli := testAction.FakeClientBuilder().Build()
		a := testAction.PrepareAction(cli, NewAction(
			testCondition, testComponent,
			Wrapper(Config[*rhtasv1.Rekor]{
				IsEnabled: func(_ *rhtasv1.Rekor) bool { return false },
			}),
		))

		g.Expect(a.CanHandle(t.Context(), instance)).To(BeFalse())
	})
}

func TestHandle_FIPSPasswordRefGuard(t *testing.T) {
	original := fipsutil.Enabled
	fipsutil.Enabled = func() bool { return true }
	t.Cleanup(func() { fipsutil.Enabled = original })

	t.Run("rejects PasswordRef as terminal error", func(t *testing.T) {
		g := NewWithT(t)
		ctx := t.Context()
		instance := testInstance(pendingConditions()...)

		cli := testAction.FakeClientBuilder().
			WithObjects(instance).
			WithStatusSubresource(instance).
			Build()

		a := testAction.PrepareAction(cli, NewAction(
			testCondition, testComponent,
			testFIPSWrapper(
				func(_ *rhtasv1.Rekor) *rhtasv1.SecretKeySelector {
					return &rhtasv1.SecretKeySelector{
						LocalObjectReference: rhtasv1.LocalObjectReference{Name: "password-secret"},
						Key:                  "password",
					}
				},
				nil,
			),
		))

		result := a.Handle(ctx, instance)

		g.Expect(result).ToNot(BeNil())
		g.Expect(result.Err).To(HaveOccurred())
		g.Expect(errors.Is(result.Err, reconcile.TerminalError(result.Err))).To(BeTrue())
		g.Expect(errors.Is(result.Err, fipsutil.ErrPasswordRefInFIPS)).To(BeTrue())
	})

	t.Run("allows nil PasswordRef in FIPS mode", func(t *testing.T) {
		g := NewWithT(t)
		ctx := t.Context()
		instance := testInstance(pendingConditions()...)

		cli := testAction.FakeClientBuilder().
			WithObjects(instance).
			WithStatusSubresource(instance).
			Build()

		a := testAction.PrepareAction(cli, NewAction(
			testCondition, testComponent,
			testFIPSWrapper(
				func(_ *rhtasv1.Rekor) *rhtasv1.SecretKeySelector {
					return nil
				},
				nil,
			),
		))

		result := a.Handle(ctx, instance)

		g.Expect(result).To(Equal(testAction.Return()))
	})
}

func TestHandle_FIPSCryptoValidation(t *testing.T) {
	original := fipsutil.Enabled
	fipsutil.Enabled = func() bool { return true }
	t.Cleanup(func() { fipsutil.Enabled = original })

	cryptoMaterialWrapper := func(secretName string) func(*rhtasv1.Rekor) *wrapper[*rhtasv1.Rekor] {
		return testFIPSWrapper(
			nil,
			func(_ context.Context, _ *rhtasv1.Rekor, c client.Client) ([]CryptoRef, error) {
				secret := &corev1.Secret{}
				if err := c.Get(context.TODO(), client.ObjectKey{Name: secretName, Namespace: testNamespace}, secret); err != nil {
					return nil, err
				}
				return []CryptoRef{{
					FieldPath: "spec.signer.keyRef",
					Data:      secret.Data["private"],
					Validate:  fipsutil.ValidatePrivateKeyPEM,
				}}, nil
			},
		)
	}

	t.Run("accepts Ed25519 key", func(t *testing.T) {
		g := NewWithT(t)
		ctx := t.Context()
		instance := testInstance(pendingConditions()...)

		keyPEM := generateEd25519KeyPEM(t)
		keySecret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: "ed25519-secret", Namespace: testNamespace},
			Data:       map[string][]byte{"private": keyPEM},
		}

		cli := testAction.FakeClientBuilder().
			WithObjects(instance, keySecret).
			WithStatusSubresource(instance).
			Build()

		a := testAction.PrepareAction(cli, NewAction(
			testCondition, testComponent,
			cryptoMaterialWrapper("ed25519-secret"),
		))

		result := a.Handle(ctx, instance)

		g.Expect(result).To(Equal(testAction.Return()))
	})

	t.Run("rejects RSA-1024 key as terminal error", func(t *testing.T) {
		g := NewWithT(t)
		ctx := t.Context()
		instance := testInstance(pendingConditions()...)

		keyPEM := generateRSAKeyPEM(t, 1024)
		keySecret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: "rsa1024-secret", Namespace: testNamespace},
			Data:       map[string][]byte{"private": keyPEM},
		}

		cli := testAction.FakeClientBuilder().
			WithObjects(instance, keySecret).
			WithStatusSubresource(instance).
			Build()

		a := testAction.PrepareAction(cli, NewAction(
			testCondition, testComponent,
			cryptoMaterialWrapper("rsa1024-secret"),
		))

		result := a.Handle(ctx, instance)

		g.Expect(result).ToNot(BeNil())
		g.Expect(result.Err).To(HaveOccurred())
		g.Expect(errors.Is(result.Err, reconcile.TerminalError(result.Err))).To(BeTrue())
		g.Expect(result.Err.Error()).To(ContainSubstring("FIPS validation failed"))

		cc := meta.FindStatusCondition(instance.Status.Conditions, testCondition)
		g.Expect(cc).ToNot(BeNil())
		g.Expect(cc.Status).To(Equal(metav1.ConditionFalse))
	})

	t.Run("skips validation when CryptoMaterial callback is nil", func(t *testing.T) {
		g := NewWithT(t)
		ctx := t.Context()
		instance := testInstance(pendingConditions()...)

		cli := testAction.FakeClientBuilder().
			WithObjects(instance).
			WithStatusSubresource(instance).
			Build()

		a := testAction.PrepareAction(cli, NewAction(
			testCondition, testComponent,
			testFIPSWrapper(nil, nil),
		))

		result := a.Handle(ctx, instance)

		g.Expect(result).To(Equal(testAction.Return()))
	})
}

func TestHandle_CryptoMaterialCallbackError(t *testing.T) {
	original := fipsutil.Enabled
	fipsutil.Enabled = func() bool { return true }
	t.Cleanup(func() { fipsutil.Enabled = original })

	t.Run("ValidationError from CryptoMaterial is terminal", func(t *testing.T) {
		g := NewWithT(t)
		ctx := t.Context()
		instance := testInstance(pendingConditions()...)

		cli := testAction.FakeClientBuilder().
			WithObjects(instance).
			WithStatusSubresource(instance).
			Build()

		a := testAction.PrepareAction(cli, NewAction(
			testCondition, testComponent,
			testFIPSWrapper(
				nil,
				func(_ context.Context, _ *rhtasv1.Rekor, _ client.Client) ([]CryptoRef, error) {
					return nil, fipsutil.NewValidationError(fmt.Errorf("invalid base64: illegal data"))
				},
			),
		))

		result := a.Handle(ctx, instance)

		g.Expect(result).ToNot(BeNil())
		g.Expect(result.Err).To(HaveOccurred())
		g.Expect(errors.Is(result.Err, reconcile.TerminalError(result.Err))).To(BeTrue())

		cc := meta.FindStatusCondition(instance.Status.Conditions, testCondition)
		g.Expect(cc).ToNot(BeNil())
		g.Expect(cc.Status).To(Equal(metav1.ConditionFalse))
	})

	t.Run("plain error from CryptoMaterial is transient", func(t *testing.T) {
		g := NewWithT(t)
		ctx := t.Context()
		instance := testInstance(pendingConditions()...)

		cli := testAction.FakeClientBuilder().
			WithObjects(instance).
			WithStatusSubresource(instance).
			Build()

		a := testAction.PrepareAction(cli, NewAction(
			testCondition, testComponent,
			testFIPSWrapper(
				nil,
				func(_ context.Context, _ *rhtasv1.Rekor, _ client.Client) ([]CryptoRef, error) {
					return nil, fmt.Errorf("secret not found")
				},
			),
		))

		result := a.Handle(ctx, instance)

		g.Expect(result).To(Equal(testAction.Continue()))
	})
}

func TestHandle_ConditionSetOnSuccess(t *testing.T) {
	original := fipsutil.Enabled
	fipsutil.Enabled = func() bool { return true }
	t.Cleanup(func() { fipsutil.Enabled = original })

	t.Run("sets condition to True on successful validation", func(t *testing.T) {
		g := NewWithT(t)
		instance := testInstance(pendingConditions()...)

		cli := testAction.FakeClientBuilder().
			WithObjects(instance).
			WithStatusSubresource(instance).
			Build()

		a := testAction.PrepareAction(cli, NewAction(
			testCondition, testComponent,
			Wrapper(Config[*rhtasv1.Rekor]{
				IsEnabled: func(_ *rhtasv1.Rekor) bool { return true },
			}),
		))

		result := a.Handle(t.Context(), instance)
		g.Expect(result).To(Equal(testAction.Return()))

		cc := meta.FindStatusCondition(instance.Status.Conditions, testCondition)
		g.Expect(cc).ToNot(BeNil())
		g.Expect(cc.Status).To(Equal(metav1.ConditionTrue))
		g.Expect(cc.Reason).To(Equal("FIPSValid"))
	})
}

package actions

import (
	"context"
	"testing"

	. "github.com/onsi/gomega"
	rhtasv1alpha1 "github.com/securesign/operator/api/v1alpha1"
	"github.com/securesign/operator/internal/action"
	"github.com/securesign/operator/internal/constants"
	"github.com/securesign/operator/internal/state"
	testAction "github.com/securesign/operator/internal/testing/action"
	"github.com/securesign/operator/internal/utils/kubernetes"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func TestEnsurePKCS11Config_CanHandle(t *testing.T) {
	tests := []struct {
		name      string
		instance  *rhtasv1alpha1.Fulcio
		canHandle bool
	}{
		{
			name: "file CA type - skip",
			instance: &rhtasv1alpha1.Fulcio{
				Spec: rhtasv1alpha1.FulcioSpec{
					Certificate: rhtasv1alpha1.FulcioCert{CAType: rhtasv1alpha1.CATypeFile},
				},
				Status: rhtasv1alpha1.FulcioStatus{
					Conditions: []metav1.Condition{
						{Type: constants.ReadyCondition, Status: metav1.ConditionFalse, Reason: state.Creating.String()},
					},
				},
			},
			canHandle: false,
		},
		{
			name: "pkcs11 type, creating state, condition not set",
			instance: &rhtasv1alpha1.Fulcio{
				Spec: rhtasv1alpha1.FulcioSpec{
					Certificate: rhtasv1alpha1.FulcioCert{CAType: rhtasv1alpha1.CATypePKCS11},
				},
				Status: rhtasv1alpha1.FulcioStatus{
					Conditions: []metav1.Condition{
						{Type: constants.ReadyCondition, Status: metav1.ConditionFalse, Reason: state.Creating.String()},
					},
				},
			},
			canHandle: true,
		},
		{
			name: "pkcs11 type, condition already true",
			instance: &rhtasv1alpha1.Fulcio{
				Spec: rhtasv1alpha1.FulcioSpec{
					Certificate: rhtasv1alpha1.FulcioCert{CAType: rhtasv1alpha1.CATypePKCS11},
				},
				Status: rhtasv1alpha1.FulcioStatus{
					Conditions: []metav1.Condition{
						{Type: constants.ReadyCondition, Status: metav1.ConditionFalse, Reason: state.Creating.String()},
						{Type: PKCS11ConfigCondition, Status: metav1.ConditionTrue, Reason: "Resolved"},
					},
				},
			},
			canHandle: false,
		},
		{
			name: "pkcs11 type, state too early",
			instance: &rhtasv1alpha1.Fulcio{
				Spec: rhtasv1alpha1.FulcioSpec{
					Certificate: rhtasv1alpha1.FulcioCert{CAType: rhtasv1alpha1.CATypePKCS11},
				},
				Status: rhtasv1alpha1.FulcioStatus{
					Conditions: []metav1.Condition{
						{Type: constants.ReadyCondition, Status: metav1.ConditionFalse, Reason: state.Pending.String()},
					},
				},
			},
			canHandle: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)
			c := testAction.FakeClientBuilder().
				WithObjects(tt.instance).
				Build()
			a := testAction.PrepareAction(c, NewEnsurePKCS11ConfigAction())
			g.Expect(a.CanHandle(context.TODO(), tt.instance)).To(Equal(tt.canHandle))
		})
	}
}

func TestEnsurePKCS11Config_Handle(t *testing.T) {
	ctx := context.TODO()

	type env struct {
		pkcs11  *rhtasv1alpha1.PKCS11Config
		objects []client.Object
	}
	type want struct {
		result *action.Result
		verify func(Gomega, *rhtasv1alpha1.Fulcio, client.WithWatch)
	}
	tests := []struct {
		name string
		env  env
		want want
	}{
		{
			name: "inline mode creates PIN secret and crypto11.conf secret",
			env: env{
				pkcs11: &rhtasv1alpha1.PKCS11Config{
					Pin:         "my-secret-pin",
					TokenLabel:  "fulcio",
					LibraryPath: "/usr/lib64/pkcs11/libsofthsm2.so",
					InitContainer: rhtasv1alpha1.PKCS11InitContainer{
						Image: "quay.io/test/softhsm-init:latest",
					},
				},
			},
			want: want{
				result: testAction.StatusUpdate(),
				verify: func(g Gomega, instance *rhtasv1alpha1.Fulcio, cli client.WithWatch) {
					g.Expect(meta.IsStatusConditionTrue(instance.Status.Conditions, PKCS11ConfigCondition)).To(BeTrue())

					sp := instance.Status.Certificate.PKCS11
					g.Expect(sp).NotTo(BeNil())
					g.Expect(sp.CredentialsRef).NotTo(BeNil())
					g.Expect(sp.CredentialsRef.Key).To(Equal("pin"))
					g.Expect(sp.PKCS11ConfigRef).NotTo(BeNil())
					g.Expect(sp.PKCS11ConfigRef.Key).To(Equal("crypto11.conf"))

					pinSecret, err := kubernetes.FindSecret(ctx, cli, "default", PKCS11CredLabel)
					g.Expect(err).NotTo(HaveOccurred())
					g.Expect(pinSecret.Name).To(Equal(sp.CredentialsRef.Name))

					confSecret, err := kubernetes.FindSecret(ctx, cli, "default", PKCS11ConfLabel)
					g.Expect(err).NotTo(HaveOccurred())
					g.Expect(confSecret.Name).To(Equal(sp.PKCS11ConfigRef.Name))
				},
			},
		},
		{
			name: "reference mode uses existing refs",
			env: env{
				pkcs11: &rhtasv1alpha1.PKCS11Config{
					CredentialsRef: &rhtasv1alpha1.SecretKeySelector{
						Key:                  "pin",
						LocalObjectReference: rhtasv1alpha1.LocalObjectReference{Name: "my-creds"},
					},
					PKCS11ConfigRef: &rhtasv1alpha1.SecretKeySelector{
						Key:                  "config.json",
						LocalObjectReference: rhtasv1alpha1.LocalObjectReference{Name: "my-config"},
					},
					InitContainer: rhtasv1alpha1.PKCS11InitContainer{
						Image: "quay.io/test/softhsm-init:latest",
					},
				},
			},
			want: want{
				result: testAction.StatusUpdate(),
				verify: func(g Gomega, instance *rhtasv1alpha1.Fulcio, _ client.WithWatch) {
					g.Expect(meta.IsStatusConditionTrue(instance.Status.Conditions, PKCS11ConfigCondition)).To(BeTrue())

					sp := instance.Status.Certificate.PKCS11
					g.Expect(sp).NotTo(BeNil())
					g.Expect(sp.CredentialsRef.Name).To(Equal("my-creds"))
					g.Expect(sp.CredentialsRef.Key).To(Equal("pin"))
					g.Expect(sp.PKCS11ConfigRef.Name).To(Equal("my-config"))
					g.Expect(sp.PKCS11ConfigRef.Key).To(Equal("config.json"))
				},
			},
		},
		{
			name: "inline mode with inlineData creates ConfigMap",
			env: env{
				pkcs11: &rhtasv1alpha1.PKCS11Config{
					Pin:         "test-pin",
					TokenLabel:  "fulcio",
					LibraryPath: "/usr/lib64/pkcs11/libsofthsm2.so",
					InitContainer: rhtasv1alpha1.PKCS11InitContainer{
						Image: "quay.io/test/softhsm-init:latest",
						Volumes: []rhtasv1alpha1.PKCS11Volume{
							{
								Name:      "softhsm-config",
								MountPath: "/etc/softhsm",
								InlineData: map[string]string{
									"softhsm2.conf": "directories.tokendir = /var/lib/hsm/tokens\n",
								},
							},
						},
					},
				},
			},
			want: want{
				result: testAction.StatusUpdate(),
				verify: func(g Gomega, instance *rhtasv1alpha1.Fulcio, cli client.WithWatch) {
					g.Expect(meta.IsStatusConditionTrue(instance.Status.Conditions, PKCS11ConfigCondition)).To(BeTrue())

					volLabel := PKCS11VolLabelPrefix + "softhsm-config"
					cm, err := kubernetes.FindConfigMap(ctx, cli, "default", volLabel)
					g.Expect(err).NotTo(HaveOccurred())
					g.Expect(cm).NotTo(BeNil())

					sp := instance.Status.Certificate.PKCS11
					g.Expect(sp.InitContainer.Volumes[0].ConfigMapName).To(Equal(cm.Name))
				},
			},
		},
		{
			name: "idempotent — existing secrets are reused",
			env: env{
				pkcs11: &rhtasv1alpha1.PKCS11Config{
					Pin:         "my-pin",
					TokenLabel:  "fulcio",
					LibraryPath: "/usr/lib64/pkcs11/libsofthsm2.so",
					InitContainer: rhtasv1alpha1.PKCS11InitContainer{
						Image: "quay.io/test/softhsm-init:latest",
					},
				},
				objects: []client.Object{
					&v1.Secret{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "existing-creds",
							Namespace: "default",
							Labels:    map[string]string{PKCS11CredLabel: "pin"},
						},
						Data: map[string][]byte{"pin": []byte("old-pin")},
					},
					&v1.Secret{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "existing-conf",
							Namespace: "default",
							Labels:    map[string]string{PKCS11ConfLabel: "crypto11.conf"},
						},
						Data: map[string][]byte{"crypto11.conf": []byte("{}")},
					},
				},
			},
			want: want{
				result: testAction.StatusUpdate(),
				verify: func(g Gomega, instance *rhtasv1alpha1.Fulcio, _ client.WithWatch) {
					sp := instance.Status.Certificate.PKCS11
					g.Expect(sp.CredentialsRef.Name).To(Equal("existing-creds"))
					g.Expect(sp.PKCS11ConfigRef.Name).To(Equal("existing-conf"))
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			instance := &rhtasv1alpha1.Fulcio{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-fulcio",
					Namespace: "default",
				},
				Spec: rhtasv1alpha1.FulcioSpec{
					Certificate: rhtasv1alpha1.FulcioCert{
						CAType: rhtasv1alpha1.CATypePKCS11,
						PKCS11: tt.env.pkcs11,
					},
				},
				Status: rhtasv1alpha1.FulcioStatus{
					Conditions: []metav1.Condition{
						{Type: constants.ReadyCondition, Status: metav1.ConditionFalse, Reason: state.Creating.String()},
					},
				},
			}

			builder := testAction.FakeClientBuilder().
				WithObjects(instance).
				WithStatusSubresource(instance)
			for _, obj := range tt.env.objects {
				builder = builder.WithObjects(obj)
			}
			c := builder.Build()

			a := testAction.PrepareAction(c, NewEnsurePKCS11ConfigAction())
			g.Expect(a.Handle(ctx, instance)).To(Equal(tt.want.result))

			found := &rhtasv1alpha1.Fulcio{}
			g.Expect(c.Get(ctx, client.ObjectKeyFromObject(instance), found)).To(Succeed())
			tt.want.verify(g, found, c)
		})
	}
}

package actions

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"maps"
	"slices"
	"time"

	rhtasv1 "github.com/securesign/operator/api/v1"
	"github.com/securesign/operator/internal/action"
	"github.com/securesign/operator/internal/constants"
	tsaUtils "github.com/securesign/operator/internal/controller/tsa/utils"
	"github.com/securesign/operator/internal/labels"
	"github.com/securesign/operator/internal/state"
	"github.com/securesign/operator/internal/utils/kubernetes"
	"github.com/securesign/operator/internal/utils/kubernetes/ensure"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	TSACertCALabel = labels.LabelNamespace + "/tsa.certchain.pem"
)

var managedAnnotations = []string{labels.LabelNamespace + "/signerConfiguration"}

type generateSigner struct {
	action.BaseAction
}

func NewGenerateSignerAction() action.Action[*rhtasv1.TimestampAuthority] {
	return &generateSigner{}
}

func (g generateSigner) Name() string {
	return "handle certificate chain"
}

func (g generateSigner) CanHandle(_ context.Context, instance *rhtasv1.TimestampAuthority) bool {
	c := meta.FindStatusCondition(instance.Status.Conditions, constants.ReadyCondition)

	switch {
	case c == nil:
		return false
	case state.FromCondition(c) < state.Pending:
		return false
	case instance.Status.Signer == nil:
		return true
	default:
		// TSASignerCondition is managed exclusively by this action.
		cc := meta.FindStatusCondition(instance.GetConditions(), TSASignerCondition)
		return cc == nil || cc.Status != metav1.ConditionTrue || instance.Generation != cc.ObservedGeneration
	}
}

func (g generateSigner) Handle(ctx context.Context, instance *rhtasv1.TimestampAuthority) *action.Result {
	var (
		err error
	)

	anno, err := g.secretAnnotations(instance.Spec.Signer)
	if err != nil {
		return g.Error(ctx, err, instance)
	}

	// Check if an operator-managed secret with matching config already exists.
	partialSecret, err := kubernetes.FindSecret(ctx, g.Client, instance.Namespace, TSACertCALabel)
	if client.IgnoreNotFound(err) != nil {
		g.Logger.Error(err, "problem with finding secret")
	}

	if partialSecret != nil {
		if equality.Semantic.DeepDerivative(anno, partialSecret.GetAnnotations()) {
			// signer is valid
			g.alignStatusFields(partialSecret.Name, instance)
			meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
				Type:               TSASignerCondition,
				Status:             metav1.ConditionTrue,
				Reason:             "Resolved", //nolint:goconst
				ObservedGeneration: instance.Generation,
			})
			return g.ReturnOnChange(g.PersistStatus)(ctx, instance)
		}

		// invalidate certificate
		if err := labels.Remove(ctx, partialSecret, g.Client, TSACertCALabel); err != nil {
			return g.Error(ctx, err, instance, metav1.Condition{
				Type:               TSASignerCondition,
				Status:             metav1.ConditionFalse,
				Reason:             state.Failure.String(),
				Message:            err.Error(),
				ObservedGeneration: instance.Generation,
			})
		}
		message := fmt.Sprintf("Removed '%s' label from %s secret", TSACertCALabel, partialSecret.Name)
		g.Recorder.Eventf(instance, nil, v1.EventTypeNormal, "CertificateSecretLabelRemoved", "LabelRemoved", message)
		g.Logger.Info(message)
	}

	// Spec changed or first run — initialize status from spec.
	instance.Status.Signer = signerStatusFromSpec(&instance.Spec.Signer)
	if state.FromInstance(instance, constants.ReadyCondition) != state.Pending {
		meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
			Type:               constants.ReadyCondition,
			Status:             metav1.ConditionFalse,
			Reason:             state.Pending.String(),
			ObservedGeneration: instance.Generation,
		})
		meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
			Type:               TSASignerCondition,
			Status:             metav1.ConditionFalse,
			Reason:             state.Creating.String(),
			ObservedGeneration: instance.Generation,
		})
		return g.ReturnOnChange(g.PersistStatus)(ctx, instance)
	}

	tsaCertChainConfig := &tsaUtils.TsaCertChainConfig{}
	if tsaUtils.IsFileType(instance) {
		tsaCertChainConfig, err = g.handleSignerKeys(instance, tsaCertChainConfig)
		if err != nil {
			g.Logger.Error(err, "error resolving keys for timestamp authority")
			meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
				Type:               TSASignerCondition,
				Status:             metav1.ConditionFalse,
				Reason:             state.Failure.String(),
				Message:            err.Error(),
				ObservedGeneration: instance.Generation,
			})
			meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
				Type:               constants.ReadyCondition,
				Status:             metav1.ConditionFalse,
				Reason:             state.Pending.String(),
				Message:            "Resolving keys",
				ObservedGeneration: instance.Generation,
			})
			if _, err := g.PersistStatus(ctx, instance); err != nil {
				return g.Error(ctx, err, instance)
			}
			// swallow error and retry
			return g.RequeueAfter(5 * time.Second)
		}
	}

	tsaCertChainConfig, err = g.handleCertificateChain(ctx, instance, tsaCertChainConfig)
	if err != nil {
		g.Logger.Error(err, "error resolving certificate chain for timestamp authority")
		meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
			Type:               TSASignerCondition,
			Status:             metav1.ConditionFalse,
			Reason:             state.Failure.String(),
			Message:            err.Error(),
			ObservedGeneration: instance.Generation,
		})
		meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
			Type:               constants.ReadyCondition,
			Status:             metav1.ConditionFalse,
			Reason:             state.Pending.String(),
			Message:            "Resolving keys",
			ObservedGeneration: instance.Generation,
		})
		if _, err := g.PersistStatus(ctx, instance); err != nil {
			return g.Error(ctx, err, instance)
		}
		// swallow error and retry
		return g.RequeueAfter(5 * time.Second)
	}

	componentLabels := labels.For(ComponentName, DeploymentName, instance.Name)
	certLabels := map[string]string{TSACertCALabel: tsaUtils.KeyCertificateChain}
	certificateChain := &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: fmt.Sprintf("tsa-signer-config-%s", instance.Name),
			Namespace:    instance.Namespace,
		},
	}

	if _, err = kubernetes.CreateOrUpdate(ctx, g.Client,
		certificateChain,
		ensure.Labels[*v1.Secret](slices.Collect(maps.Keys(componentLabels)), componentLabels),
		ensure.Labels[*v1.Secret](slices.Collect(maps.Keys(certLabels)), certLabels),
		ensure.Annotations[*v1.Secret](managedAnnotations, anno),
		kubernetes.EnsureSecretData(true, tsaCertChainConfig.ToMap()),
	); err != nil {
		return g.Error(ctx, fmt.Errorf("could not create signer secret: %w", err), instance,
			metav1.Condition{
				Type:               TSASignerCondition,
				Status:             metav1.ConditionFalse,
				Reason:             state.Failure.String(),
				Message:            err.Error(),
				ObservedGeneration: instance.Generation,
			})
	}

	g.Recorder.Eventf(instance, certificateChain, v1.EventTypeNormal, "TSACertUpdated", "Updated", "TSA certificate secret updated")

	g.alignStatusFields(certificateChain.Name, instance)
	meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
		Type:               TSASignerCondition,
		Status:             metav1.ConditionTrue,
		Reason:             "Resolved",
		ObservedGeneration: instance.Generation,
	})
	return g.ReturnOnChange(g.PersistStatus)(ctx, instance)
}

func (g generateSigner) handleSignerKeys(instance *rhtasv1.TimestampAuthority, config *tsaUtils.TsaCertChainConfig) (*tsaUtils.TsaCertChainConfig, error) {
	if instance.Spec.Signer.File != nil {
		if instance.Spec.Signer.File.PrivateKeyRef != nil {
			key, err := kubernetes.GetSecretData(g.Client, instance.Namespace, instance.Spec.Signer.File.PrivateKeyRef)
			if err != nil {
				return nil, err
			}
			config.LeafPrivateKey = key

			if ref := instance.Spec.Signer.File.PasswordRef; ref != nil {
				password, err := kubernetes.GetSecretData(g.Client, instance.Namespace, ref)
				if err != nil {
					return nil, err
				}
				config.LeafPrivateKeyPassword = password
			}
		}

		if ref := instance.Spec.Signer.CertificateChain.CertificateChainRef; ref == nil {
			key, err := ecdsa.GenerateKey(elliptic.P384(), rand.Reader)
			if err != nil {
				return nil, err
			}
			rootCAPrivKey, err := tsaUtils.CreatePrivateKey(key)
			if err != nil {
				return nil, err
			}
			config.RootPrivateKey = rootCAPrivKey
		}

	} else {
		if instance.Spec.Signer.CertificateChain.RootCA != nil {
			if ref := instance.Spec.Signer.CertificateChain.RootCA.PrivateKeyRef; ref != nil {
				key, err := kubernetes.GetSecretData(g.Client, instance.Namespace, ref)
				if err != nil {
					return nil, err
				}
				config.RootPrivateKey = key

				if ref := instance.Spec.Signer.CertificateChain.RootCA.PasswordRef; ref != nil {
					password, err := kubernetes.GetSecretData(g.Client, instance.Namespace, ref)
					if err != nil {
						return nil, err
					}
					config.RootPrivateKeyPassword = password
				}
			} else {
				key, err := ecdsa.GenerateKey(elliptic.P384(), rand.Reader)
				if err != nil {
					return nil, err
				}
				rootCAPrivKey, err := tsaUtils.CreatePrivateKey(key)
				if err != nil {
					return nil, err
				}
				config.RootPrivateKey = rootCAPrivKey
			}
		}

		for _, intermediateCA := range instance.Spec.Signer.CertificateChain.IntermediateCA {
			if ref := intermediateCA.PrivateKeyRef; ref != nil {
				key, err := kubernetes.GetSecretData(g.Client, instance.Namespace, ref)
				if err != nil {
					return nil, err
				}
				config.IntermediatePrivateKeys = append(config.IntermediatePrivateKeys, key)

				if ref := intermediateCA.PasswordRef; ref != nil {
					password, err := kubernetes.GetSecretData(g.Client, instance.Namespace, ref)
					if err != nil {
						return nil, err
					}
					config.IntermediatePrivateKeyPasswords = append(config.IntermediatePrivateKeyPasswords, password)
				} else {
					config.IntermediatePrivateKeyPasswords = append(config.IntermediatePrivateKeyPasswords, []byte(""))
				}
			} else {
				config.IntermediatePrivateKeyPasswords = append(config.IntermediatePrivateKeyPasswords, nil)
				key, err := ecdsa.GenerateKey(elliptic.P384(), rand.Reader)
				if err != nil {
					return nil, err
				}
				interCAPrivKey, err := tsaUtils.CreatePrivateKey(key)
				if err != nil {
					return nil, err
				}
				config.IntermediatePrivateKeys = append(config.IntermediatePrivateKeys, interCAPrivKey)
			}
		}

		if instance.Spec.Signer.CertificateChain.LeafCA != nil {
			if ref := instance.Spec.Signer.CertificateChain.LeafCA.PrivateKeyRef; ref != nil {
				key, err := kubernetes.GetSecretData(g.Client, instance.Namespace, ref)
				if err != nil {
					return nil, err
				}
				config.LeafPrivateKey = key

				if ref := instance.Spec.Signer.CertificateChain.LeafCA.PasswordRef; ref != nil {
					password, err := kubernetes.GetSecretData(g.Client, instance.Namespace, ref)
					if err != nil {
						return nil, err
					}
					config.LeafPrivateKeyPassword = password
				}
			} else {
				key, err := ecdsa.GenerateKey(elliptic.P384(), rand.Reader)
				if err != nil {
					return nil, err
				}
				leafCAPrivKey, err := tsaUtils.CreatePrivateKey(key)
				if err != nil {
					return nil, err
				}
				config.LeafPrivateKey = leafCAPrivKey
			}
		}
	}
	return config, nil
}

func (g generateSigner) handleCertificateChain(ctx context.Context, instance *rhtasv1.TimestampAuthority, config *tsaUtils.TsaCertChainConfig) (*tsaUtils.TsaCertChainConfig, error) {
	if ref := instance.Spec.Signer.CertificateChain.CertificateChainRef; ref != nil {
		certificateChain, err := kubernetes.GetSecretData(g.Client, instance.Namespace, ref)
		if err != nil {
			return nil, err
		}
		config.CertificateChain = certificateChain
	} else {
		if instance.Spec.Signer.CertificateChain.RootCA != nil && instance.Spec.Signer.CertificateChain.LeafCA != nil {
			certificateChain, err := tsaUtils.CreateTSACertChain(ctx, instance, DeploymentName, g.Client, config)
			if err != nil {
				return nil, err
			}
			config.CertificateChain = certificateChain
		}
	}
	return config, nil
}

func (g generateSigner) alignStatusFields(secretName string, instance *rhtasv1.TimestampAuthority) {
	instance.Status.Signer = signerStatusFromSpec(&instance.Spec.Signer)

	// Default to File-based signer when no signer type (File/Tink/KMS) and no
	// external cert chain are configured.
	if instance.Spec.Signer.File == nil && instance.Spec.Signer.CertificateChain.CertificateChainRef == nil {
		instance.Status.Signer.FileSigner = new(rhtasv1.FileSignerStatus)
	}

	if instance.Status.Signer.CertificateChainRef == nil {
		instance.Status.Signer.CertificateChainRef = &rhtasv1.SecretKeySelector{
			Key: tsaUtils.KeyCertificateChain,
			LocalObjectReference: rhtasv1.LocalObjectReference{
				Name: secretName,
			},
		}
	}

	if instance.Status.Signer.FileSigner != nil && instance.Status.Signer.FileSigner.PrivateKeyRef == nil {
		instance.Status.Signer.FileSigner.PrivateKeyRef = &rhtasv1.SecretKeySelector{
			Key: tsaUtils.KeyLeafPrivateKey,
			LocalObjectReference: rhtasv1.LocalObjectReference{
				Name: secretName,
			},
		}
	}
}

func signerStatusFromSpec(signer *rhtasv1.TimestampAuthoritySigner) *rhtasv1.TimestampAuthoritySignerStatus {
	status := &rhtasv1.TimestampAuthoritySignerStatus{
		CertificateChainRef: signer.CertificateChain.CertificateChainRef.DeepCopy(),
	}
	if signer.File != nil {
		status.FileSigner = &rhtasv1.FileSignerStatus{
			PrivateKeyRef: signer.File.PrivateKeyRef.DeepCopy(),
		}
		if signer.File.PasswordRef != nil { //nolint:staticcheck
			status.FileSigner.PasswordRef = signer.File.PasswordRef.DeepCopy() //nolint:staticcheck
		}
	}
	return status
}

func (g generateSigner) secretAnnotations(signerConfig rhtasv1.TimestampAuthoritySigner) (map[string]string, error) {
	annotations := make(map[string]string)
	bytes, err := json.Marshal(signerConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal signer configuration: %w", err)
	}
	annotations[labels.LabelNamespace+"/signerConfiguration"] = string(bytes)
	return annotations, nil
}

package v1alpha1

import (
	rhtasv1 "github.com/securesign/operator/api/v1"
	utilconversion "github.com/securesign/operator/internal/conversion"
	apiconversion "k8s.io/apimachinery/pkg/conversion"
	"sigs.k8s.io/controller-runtime/pkg/conversion"
)

func (src *Tuf) ConvertTo(dstRaw conversion.Hub) error {
	dst := dstRaw.(*rhtasv1.Tuf)
	if err := Convert_v1alpha1_Tuf_To_v1_Tuf(src, dst, nil); err != nil {
		return err
	}
	restored := &rhtasv1.Tuf{}
	if ok, err := utilconversion.UnmarshalData(src, restored); err != nil || !ok {
		return err
	}
	dst.Spec.ImagePullSecrets = restored.Spec.ImagePullSecrets
	dst.Spec.TrustedCA = restored.Spec.TrustedCA

	restoreBindingRef(dst.Spec.Rekor, restored.Spec.Rekor)
	// v1alpha1 inject prefix into URL - we need to restore empty URL to allow ref resolution
	if len(dst.Spec.Ctlog) > 0 && len(restored.Spec.Ctlog) > 0 &&
		restored.Spec.Ctlog[0].URL == "" && dst.Spec.Ctlog[0].URL == "///trusted-artifact-signer" { //nolint:goconst
		dst.Spec.Ctlog[0].URL = ""
	}
	restoreBindingRef(dst.Spec.Ctlog, restored.Spec.Ctlog)
	if len(dst.Spec.Fulcio) > 0 && len(restored.Spec.Fulcio) > 0 {
		if dst.Spec.Fulcio[0].URL == "" {
			dst.Spec.Fulcio[0].Ref = restored.Spec.Fulcio[0].Ref
		}
		dst.Spec.Fulcio[0].OIDCIssuers = restored.Spec.Fulcio[0].OIDCIssuers
	}
	if dst.Spec.Tsa != nil && restored.Spec.Tsa != nil {
		restoreBindingRef(*dst.Spec.Tsa, *restored.Spec.Tsa)
	}

	dst.Spec.PodExtensions = restored.Spec.PodExtensions
	return nil
}

// restoreBindingRef restores bindings[0].Ref from restored[0].Ref when bindings[0]
// has no explicit URL. v1alpha1 has no ref concept at all, so Ref information only
// survives via the conversion-data annotation.
func restoreBindingRef(bindings, restored []rhtasv1.TrustRootBinding) {
	if len(bindings) == 0 || len(restored) == 0 {
		return
	}
	if bindings[0].URL == "" {
		bindings[0].Ref = restored[0].Ref
	}
}

func (dst *Tuf) ConvertFrom(srcRaw conversion.Hub) error {
	src := srcRaw.(*rhtasv1.Tuf)
	if err := Convert_v1_Tuf_To_v1alpha1_Tuf(src, dst, nil); err != nil {
		return err
	}
	return utilconversion.MarshalData(src, dst)
}

func Convert_v1alpha1_TufSpec_To_v1_TufSpec(in *TufSpec, out *rhtasv1.TufSpec, s apiconversion.Scope) error {
	if err := autoConvert_v1alpha1_TufSpec_To_v1_TufSpec(in, out, s); err != nil {
		return err
	}
	if err := Convert_v1alpha1_ExternalAccess_To_v1_Ingress(&in.ExternalAccess, &out.Ingress, s); err != nil {
		return err
	}

	// secretRefs is keyed by TufKey name so it doubles as the "was this component
	// present in Keys at all" signal. A present-with-nil-SecretRef entry still
	// registers a (nil-valued) map key, distinct from the name being absent.
	secretRefs := make(map[string]*rhtasv1.SecretKeySelector, len(in.Keys))
	for _, key := range in.Keys {
		secretRefs[key.Name] = convertSecretKeySelectorTo(key.SecretRef)
	}

	rekorURL, err := buildURL(in.Rekor.Address, in.Rekor.Port, "")
	if err != nil {
		return err
	}
	out.Rekor = []rhtasv1.TrustRootBinding{{ServiceReference: rhtasv1.ServiceReference{URL: rekorURL}, SecretRef: secretRefs[rhtasv1.TufKeyRekor]}}

	ctlogURL, err := buildURL(in.Ctlog.Address, in.Ctlog.Port, in.Ctlog.Prefix)
	if err != nil {
		return err
	}
	out.Ctlog = []rhtasv1.TrustRootBinding{{ServiceReference: rhtasv1.ServiceReference{URL: ctlogURL}, SecretRef: secretRefs[rhtasv1.TufKeyCTFE]}}

	fulcioURL, err := buildURL(in.Fulcio.Address, in.Fulcio.Port, "")
	if err != nil {
		return err
	}
	out.Fulcio = []rhtasv1.TrustRootBindingWithOIDC{{TrustRootBinding: rhtasv1.TrustRootBinding{ServiceReference: rhtasv1.ServiceReference{URL: fulcioURL}, SecretRef: secretRefs[rhtasv1.TufKeyFulcio]}}}

	// TSA is only included in the trust root if tsa.certchain.pem appears in Keys at
	// all. A hand-authored v1alpha1 Tuf CR that dropped it from Keys is how
	// "excluded" is expressed in v1alpha1 terms.
	if tsaSecretRef, tsaConfigured := secretRefs[rhtasv1.TufKeyTSA]; tsaConfigured {
		tsaURL, err := buildURL(in.Tsa.Address, in.Tsa.Port, "")
		if err != nil {
			return err
		}
		out.Tsa = &[]rhtasv1.TrustRootBinding{{ServiceReference: rhtasv1.ServiceReference{URL: tsaURL}, SecretRef: tsaSecretRef}}
	} else {
		out.Tsa = nil
	}

	return nil
}

func Convert_v1_TufSpec_To_v1alpha1_TufSpec(in *rhtasv1.TufSpec, out *TufSpec, s apiconversion.Scope) error {
	if err := autoConvert_v1_TufSpec_To_v1alpha1_TufSpec(in, out, s); err != nil {
		return err
	}
	if err := Convert_v1_Ingress_To_v1alpha1_ExternalAccess(&in.Ingress, &out.ExternalAccess, s); err != nil {
		return err
	}

	keys := make([]TufKey, 0, 4)

	if len(in.Rekor) > 0 {
		if err := serviceReferenceToAddressPort(&in.Rekor[0].ServiceReference, &out.Rekor.Address, &out.Rekor.Port); err != nil {
			return err
		}
		keys = append(keys, TufKey{Name: rhtasv1.TufKeyRekor, SecretRef: convertSecretKeySelectorFrom(in.Rekor[0].SecretRef)})
	} else {
		keys = append(keys, TufKey{Name: rhtasv1.TufKeyRekor})
	}

	if len(in.Ctlog) > 0 {
		if in.Ctlog[0].URL != "" {
			base, prefix, err := splitURLPath(in.Ctlog[0].URL)
			if err != nil {
				return err
			}
			if prefix != "" {
				out.Ctlog.Prefix = prefix
			}
			ref := rhtasv1.ServiceReference{URL: base}
			if err := serviceReferenceToAddressPort(&ref, &out.Ctlog.Address, &out.Ctlog.Port); err != nil {
				return err
			}
		}
		keys = append(keys, TufKey{Name: rhtasv1.TufKeyCTFE, SecretRef: convertSecretKeySelectorFrom(in.Ctlog[0].SecretRef)})
	} else {
		keys = append(keys, TufKey{Name: rhtasv1.TufKeyCTFE})
	}

	if len(in.Fulcio) > 0 {
		if err := serviceReferenceToAddressPort(&in.Fulcio[0].ServiceReference, &out.Fulcio.Address, &out.Fulcio.Port); err != nil {
			return err
		}
		keys = append(keys, TufKey{Name: rhtasv1.TufKeyFulcio, SecretRef: convertSecretKeySelectorFrom(in.Fulcio[0].SecretRef)})
	} else {
		keys = append(keys, TufKey{Name: rhtasv1.TufKeyFulcio})
	}

	if in.Tsa != nil && len(*in.Tsa) > 0 {
		tsa := (*in.Tsa)[0]
		if err := serviceReferenceToAddressPort(&tsa.ServiceReference, &out.Tsa.Address, &out.Tsa.Port); err != nil {
			return err
		}
		keys = append(keys, TufKey{Name: rhtasv1.TufKeyTSA, SecretRef: convertSecretKeySelectorFrom(tsa.SecretRef)})
	}
	// else: TSA excluded

	out.Keys = keys
	return nil
}

func convertSecretKeySelectorTo(in *SecretKeySelector) *rhtasv1.SecretKeySelector {
	if in == nil {
		return nil
	}
	return &rhtasv1.SecretKeySelector{
		LocalObjectReference: rhtasv1.LocalObjectReference{Name: in.Name},
		Key:                  in.Key,
	}
}

func convertSecretKeySelectorFrom(in *rhtasv1.SecretKeySelector) *SecretKeySelector {
	if in == nil {
		return nil
	}
	return &SecretKeySelector{
		LocalObjectReference: LocalObjectReference{Name: in.Name},
		Key:                  in.Key,
	}
}

func Convert_v1alpha1_TufPvc_To_v1_Pvc(in *TufPvc, out *rhtasv1.Pvc, s apiconversion.Scope) error {
	pvc := Pvc(*in)
	return Convert_v1alpha1_Pvc_To_v1_Pvc(&pvc, out, s)
}

func Convert_v1_Pvc_To_v1alpha1_TufPvc(in *rhtasv1.Pvc, out *TufPvc, s apiconversion.Scope) error {
	var pvc Pvc
	if err := Convert_v1_Pvc_To_v1alpha1_Pvc(in, &pvc, s); err != nil {
		return err
	}
	*out = TufPvc(pvc)
	return nil
}

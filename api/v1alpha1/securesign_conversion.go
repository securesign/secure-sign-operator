package v1alpha1

import (
	rhtasv1 "github.com/securesign/operator/api/v1"
	"github.com/securesign/operator/internal/controller/trillian/dbsecret"
	utilconversion "github.com/securesign/operator/internal/conversion"
	"github.com/securesign/operator/internal/migration"
	"k8s.io/apimachinery/pkg/api/equality"
	apiconversion "k8s.io/apimachinery/pkg/conversion"
	"sigs.k8s.io/controller-runtime/pkg/conversion"
)

func Convert_v1alpha1_SecuresignTSAStatus_To_v1_SecuresignTSAStatus(in *SecuresignTSAStatus, out *rhtasv1.SecuresignTSAStatus, s apiconversion.Scope) error {
	if err := autoConvert_v1alpha1_SecuresignTSAStatus_To_v1_SecuresignTSAStatus(in, out, s); err != nil {
		return err
	}
	if out.Url != "" {
		var err error
		if out.Url, err = buildURL(out.Url, nil, rhtasv1.TimestampPath); err != nil {
			return err
		}
	}
	return nil
}

func Convert_v1_SecuresignTSAStatus_To_v1alpha1_SecuresignTSAStatus(in *rhtasv1.SecuresignTSAStatus, out *SecuresignTSAStatus, s apiconversion.Scope) error {
	if err := autoConvert_v1_SecuresignTSAStatus_To_v1alpha1_SecuresignTSAStatus(in, out, s); err != nil {
		return err
	}
	if out.Url != "" {
		var err error
		if out.Url, _, err = splitURLPath(out.Url); err != nil {
			return err
		}
	}
	return nil
}

func (src *Securesign) ConvertTo(dstRaw conversion.Hub) error {
	dst := dstRaw.(*rhtasv1.Securesign)
	if err := Convert_v1alpha1_Securesign_To_v1_Securesign(src, dst, nil); err != nil {
		return err
	}

	if err := migration.Set(dst, MigrationSearchUIData, src.Spec.Rekor.RekorSearchUI); err != nil {
		return err
	}

	restored := &rhtasv1.Securesign{}
	if ok, err := utilconversion.UnmarshalData(src, restored); err != nil || !ok {
		return err
	}
	dst.Spec.Fulcio.ImagePullSecrets = restored.Spec.Fulcio.ImagePullSecrets
	dst.Spec.Fulcio.Monitoring.ServiceMonitor = restored.Spec.Fulcio.Monitoring.ServiceMonitor
	dst.Spec.Fulcio.Signer.Type = restored.Spec.Fulcio.Signer.Type
	if dst.Spec.Fulcio.Signer.File == nil {
		dst.Spec.Fulcio.Signer.File = restored.Spec.Fulcio.Signer.File
	}
	if dst.Spec.Fulcio.Signer.Kms == nil {
		dst.Spec.Fulcio.Signer.Kms = restored.Spec.Fulcio.Signer.Kms
	}
	// v1alpha1 inject prefix into URL - we need to restore empty URL to allow ref resolution
	if restored.Spec.Fulcio.Ctlog.URL == "" && dst.Spec.Fulcio.Ctlog.URL == "///trusted-artifact-signer" { //nolint:goconst
		dst.Spec.Fulcio.Ctlog.URL = ""
	}
	if dst.Spec.Fulcio.Ctlog.URL == "" {
		dst.Spec.Fulcio.Ctlog.Ref = restored.Spec.Fulcio.Ctlog.Ref
	}
	dst.Spec.Fulcio.PodExtensions = restored.Spec.Fulcio.PodExtensions
	dst.Spec.Fulcio.Auth = restored.Spec.Fulcio.Auth
	dst.Spec.Fulcio.Signer.PKCS11 = restored.Spec.Fulcio.Signer.PKCS11
	dst.Spec.Ctlog.ImagePullSecrets = restored.Spec.Ctlog.ImagePullSecrets
	dst.Spec.Ctlog.TrustedCA = restored.Spec.Ctlog.TrustedCA
	dst.Spec.Ctlog.Monitoring.ServiceMonitor = restored.Spec.Ctlog.Monitoring.ServiceMonitor
	dst.Spec.Ctlog.Prefix = restored.Spec.Ctlog.Prefix
	dst.Spec.Ctlog.Sharding = restored.Spec.Ctlog.Sharding
	dst.Spec.Ctlog.Signer.Type = restored.Spec.Ctlog.Signer.Type
	// If original v1 had File=&{} (empty struct), preserve it
	if dst.Spec.Ctlog.Signer.File == nil && restored.Spec.Ctlog.Signer.File != nil {
		emptyFile := &rhtasv1.CTlogFile{}
		if equality.Semantic.DeepEqual(restored.Spec.Ctlog.Signer.File, emptyFile) {
			dst.Spec.Ctlog.Signer.File = &rhtasv1.CTlogFile{}
		}
	}
	if dst.Spec.Ctlog.Trillian.URL == "" {
		dst.Spec.Ctlog.Trillian.Ref = restored.Spec.Ctlog.Trillian.Ref
	}
	if dst.Spec.Ctlog.Monitoring.Tuf.URL == "" {
		dst.Spec.Ctlog.Monitoring.Tuf.Ref = restored.Spec.Ctlog.Monitoring.Tuf.Ref
	}
	dst.Spec.Ctlog.PodExtensions = restored.Spec.Ctlog.PodExtensions
	dst.Spec.Ctlog.Auth = restored.Spec.Ctlog.Auth
	dst.Spec.Ctlog.Ingress = restored.Spec.Ctlog.Ingress
	dst.Spec.Ctlog.Signer.PKCS11 = restored.Spec.Ctlog.Signer.PKCS11
	dst.Spec.Rekor.ImagePullSecrets = restored.Spec.Rekor.ImagePullSecrets
	dst.Spec.Rekor.Monitoring.ServiceMonitor = restored.Spec.Rekor.Monitoring.ServiceMonitor
	dst.Spec.Rekor.PodExtensions = restored.Spec.Rekor.PodExtensions
	if dst.Spec.Rekor.Trillian.URL == "" {
		dst.Spec.Rekor.Trillian.Ref = restored.Spec.Rekor.Trillian.Ref
	}
	if dst.Spec.Rekor.Monitoring.Tuf.URL == "" {
		dst.Spec.Rekor.Monitoring.Tuf.Ref = restored.Spec.Rekor.Monitoring.Tuf.Ref
	}
	dst.Spec.Trillian.ImagePullSecrets = restored.Spec.Trillian.ImagePullSecrets
	dst.Spec.Trillian.Monitoring.ServiceMonitor = restored.Spec.Trillian.Monitoring.ServiceMonitor
	dst.Spec.Trillian.PodExtensions = restored.Spec.Trillian.PodExtensions
	if src.Spec.Trillian.Db.DatabaseSecretRef != nil {
		v1Ref := &rhtasv1.LocalObjectReference{Name: src.Spec.Trillian.Db.DatabaseSecretRef.Name}
		auth := dbsecret.DbSecretToAuth(v1Ref)
		if dst.Spec.Trillian.Auth == nil {
			dst.Spec.Trillian.Auth = auth
		} else {
			dst.Spec.Trillian.Auth = mergeAuths(dst.Spec.Trillian.Auth, auth)
		}
	}

	dst.Spec.Tuf.ImagePullSecrets = restored.Spec.Tuf.ImagePullSecrets
	dst.Spec.Tuf.TrustedCA = restored.Spec.Tuf.TrustedCA
	dst.Spec.Tuf.PodExtensions = restored.Spec.Tuf.PodExtensions
	restoreBindingRef(dst.Spec.Tuf.Rekor, restored.Spec.Tuf.Rekor)
	if len(dst.Spec.Tuf.Fulcio) > 0 && len(restored.Spec.Tuf.Fulcio) > 0 {
		if dst.Spec.Tuf.Fulcio[0].URL == "" {
			dst.Spec.Tuf.Fulcio[0].Ref = restored.Spec.Tuf.Fulcio[0].Ref
		}
		dst.Spec.Tuf.Fulcio[0].OIDCIssuers = restored.Spec.Tuf.Fulcio[0].OIDCIssuers
	}
	// v1alpha1 inject prefix into URL - we need to restore empty URL to allow ref resolution
	if len(dst.Spec.Tuf.Ctlog) > 0 && len(restored.Spec.Tuf.Ctlog) > 0 &&
		restored.Spec.Tuf.Ctlog[0].URL == "" && dst.Spec.Tuf.Ctlog[0].URL == "///trusted-artifact-signer" { //nolint:goconst
		dst.Spec.Tuf.Ctlog[0].URL = ""
	}
	restoreBindingRef(dst.Spec.Tuf.Ctlog, restored.Spec.Tuf.Ctlog)
	if dst.Spec.Tuf.Tsa != nil && restored.Spec.Tuf.Tsa != nil {
		restoreBindingRef(*dst.Spec.Tuf.Tsa, *restored.Spec.Tuf.Tsa)
	}
	if dst.Spec.TimestampAuthority != nil && restored.Spec.TimestampAuthority != nil {
		dst.Spec.TimestampAuthority.ImagePullSecrets = restored.Spec.TimestampAuthority.ImagePullSecrets
		dst.Spec.TimestampAuthority.Monitoring.ServiceMonitor = restored.Spec.TimestampAuthority.Monitoring.ServiceMonitor
		dst.Spec.TimestampAuthority.PodExtensions = restored.Spec.TimestampAuthority.PodExtensions
		// restore also the auth from annotation for case where no KMS or Tink is set
		dst.Spec.TimestampAuthority.Auth = mergeAuths(dst.Spec.TimestampAuthority.Auth, restored.Spec.TimestampAuthority.Auth)
	}
	return nil
}

func (dst *Securesign) ConvertFrom(srcRaw conversion.Hub) error {
	src := srcRaw.(*rhtasv1.Securesign)
	if err := Convert_v1_Securesign_To_v1alpha1_Securesign(src, dst, nil); err != nil {
		return err
	}

	var searchUI RekorSearchUI
	if ok, err := migration.Pop(src, MigrationSearchUIData, &searchUI); err != nil {
		return err
	} else if ok {
		dst.Spec.Rekor.RekorSearchUI = searchUI
	}

	return utilconversion.MarshalData(src, dst)
}

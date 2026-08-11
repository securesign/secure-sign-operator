package v1alpha1

import (
	rhtasv1 "github.com/securesign/operator/api/v1"
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
	// If original v1 had File=&{} (empty struct), preserve it
	if dst.Spec.Fulcio.Signer.File == nil && restored.Spec.Fulcio.Signer.File != nil {
		emptyFile := &rhtasv1.FulcioFile{}
		if equality.Semantic.DeepEqual(restored.Spec.Fulcio.Signer.File, emptyFile) {
			dst.Spec.Fulcio.Signer.File = &rhtasv1.FulcioFile{}
		}
	}
	if dst.Spec.Fulcio.Ctlog.URL == "" {
		dst.Spec.Fulcio.Ctlog.Ref = restored.Spec.Fulcio.Ctlog.Ref
	}
	dst.Spec.Ctlog.ImagePullSecrets = restored.Spec.Ctlog.ImagePullSecrets
	dst.Spec.Ctlog.TrustedCA = restored.Spec.Ctlog.TrustedCA
	dst.Spec.Ctlog.Monitoring.ServiceMonitor = restored.Spec.Ctlog.Monitoring.ServiceMonitor
	dst.Spec.Ctlog.Prefix = restored.Spec.Ctlog.Prefix
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
	dst.Spec.Rekor.ImagePullSecrets = restored.Spec.Rekor.ImagePullSecrets
	dst.Spec.Rekor.Monitoring.ServiceMonitor = restored.Spec.Rekor.Monitoring.ServiceMonitor
	if dst.Spec.Rekor.Trillian.URL == "" {
		dst.Spec.Rekor.Trillian.Ref = restored.Spec.Rekor.Trillian.Ref
	}
	dst.Spec.Trillian.ImagePullSecrets = restored.Spec.Trillian.ImagePullSecrets
	dst.Spec.Trillian.Monitoring.ServiceMonitor = restored.Spec.Trillian.Monitoring.ServiceMonitor
	dst.Spec.Tuf.ImagePullSecrets = restored.Spec.Tuf.ImagePullSecrets
	dst.Spec.Tuf.TrustedCA = restored.Spec.Tuf.TrustedCA
	if dst.Spec.Tuf.Rekor.URL == "" {
		dst.Spec.Tuf.Rekor.Ref = restored.Spec.Tuf.Rekor.Ref
	}
	if dst.Spec.Tuf.Fulcio.URL == "" {
		dst.Spec.Tuf.Fulcio.Ref = restored.Spec.Tuf.Fulcio.Ref
	}
	dst.Spec.Tuf.Fulcio.OIDCIssuers = restored.Spec.Tuf.Fulcio.OIDCIssuers
	if dst.Spec.Tuf.Ctlog.URL == "" {
		dst.Spec.Tuf.Ctlog.Ref = restored.Spec.Tuf.Ctlog.Ref
	}
	if dst.Spec.Tuf.Tsa.URL == "" {
		dst.Spec.Tuf.Tsa.Ref = restored.Spec.Tuf.Tsa.Ref
	}
	if dst.Spec.TimestampAuthority != nil && restored.Spec.TimestampAuthority != nil {
		dst.Spec.TimestampAuthority.ImagePullSecrets = restored.Spec.TimestampAuthority.ImagePullSecrets
		dst.Spec.TimestampAuthority.Monitoring.ServiceMonitor = restored.Spec.TimestampAuthority.Monitoring.ServiceMonitor
		// restore also the auth from annotation for case where no KMS or Tink is set
		dst.Spec.TimestampAuthority.Signer.Auth = mergeAuths(dst.Spec.TimestampAuthority.Signer.Auth, restored.Spec.TimestampAuthority.Signer.Auth)
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

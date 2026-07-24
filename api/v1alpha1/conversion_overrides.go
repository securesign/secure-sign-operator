package v1alpha1

import (
	"net/url"

	v1 "github.com/securesign/operator/api/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	apiconversion "k8s.io/apimachinery/pkg/conversion"
)

func urlWithPath(rawUrl, path string) (string, error) {
	u, err := url.Parse(rawUrl)
	if err != nil {
		return "", err
	}
	u.Path = path
	return u.String(), nil
}

func urlWithoutPath(rawUrl string) (string, error) {
	u, err := url.Parse(rawUrl)
	if err != nil {
		return "", err
	}
	return u.Scheme + "://" + u.Host, nil
}

// MonitoringConfig: v1 splits Enabled into Metrics.Enabled + ServiceMonitor.Enabled.
// Lossless round-trip is guaranteed by MarshalData/UnmarshalData in ConvertTo/ConvertFrom.

func Convert_v1alpha1_MonitoringConfig_To_v1_MonitoringConfig(in *MonitoringConfig, out *v1.MonitoringConfig, s apiconversion.Scope) error {
	if err := metav1.Convert_bool_To_Pointer_bool(&in.Enabled, &out.Metrics.Enabled, s); err != nil {
		return err
	}
	if err := metav1.Convert_bool_To_Pointer_bool(&in.Enabled, &out.ServiceMonitor.Enabled, s); err != nil {
		return err
	}
	return nil
}

func Convert_v1_MonitoringConfig_To_v1alpha1_MonitoringConfig(in *v1.MonitoringConfig, out *MonitoringConfig, s apiconversion.Scope) error {
	if err := metav1.Convert_Pointer_bool_To_bool(&in.Metrics.Enabled, &out.Enabled, s); err != nil {
		return err
	}
	return nil
}

func Convert_v1alpha1_TlogMonitoring_To_v1_TlogMonitoring(in *TlogMonitoring, out *v1.TlogMonitoring, s apiconversion.Scope) error {
	if err := metav1.Convert_bool_To_Pointer_bool(&in.Enabled, &out.Enabled, s); err != nil {
		return err
	}
	if in.Interval.Duration != 0 {
		interval := in.Interval
		out.Interval = &interval
	}
	return nil
}

func Convert_v1_TlogMonitoring_To_v1alpha1_TlogMonitoring(in *v1.TlogMonitoring, out *TlogMonitoring, s apiconversion.Scope) error {
	if err := metav1.Convert_Pointer_bool_To_bool(&in.Enabled, &out.Enabled, s); err != nil {
		return err
	}
	if err := metav1.Convert_Pointer_v1_Duration_To_v1_Duration(&in.Interval, &out.Interval, s); err != nil {
		return err
	}
	return nil
}

// ExternalAccess (v1alpha1) was renamed to Ingress (v1), with
// RouteSelectorLabels renamed to Labels; Enabled stays bool vs *bool.

func Convert_v1alpha1_ExternalAccess_To_v1_Ingress(in *ExternalAccess, out *v1.Ingress, s apiconversion.Scope) error {
	if err := metav1.Convert_bool_To_Pointer_bool(&in.Enabled, &out.Enabled, s); err != nil {
		return err
	}
	out.Host = in.Host
	out.Labels = in.RouteSelectorLabels
	return nil
}

func Convert_v1_Ingress_To_v1alpha1_ExternalAccess(in *v1.Ingress, out *ExternalAccess, s apiconversion.Scope) error {
	if err := metav1.Convert_Pointer_bool_To_bool(&in.Enabled, &out.Enabled, s); err != nil {
		return err
	}
	out.Host = in.Host
	out.RouteSelectorLabels = in.Labels
	return nil
}

// Convert_v1alpha1_FulcioCert_To_v1_FulcioCert manually converts FulcioCert.
// v1alpha1 does not have CAType or PKCS11 fields — they are restored from
// MarshalData annotations in ConvertTo.
func Convert_v1alpha1_FulcioCert_To_v1_FulcioCert(in *FulcioCert, out *v1.FulcioCert, s apiconversion.Scope) error {
	return autoConvert_v1alpha1_FulcioCert_To_v1_FulcioCert(in, out, s)
}

// Convert_v1_FulcioCert_To_v1alpha1_FulcioCert manually converts FulcioCert from v1 to v1alpha1.
// CAType and PKCS11 fields are v1-only and intentionally dropped here.
// They are preserved via MarshalData annotation in ConvertFrom.
func Convert_v1_FulcioCert_To_v1alpha1_FulcioCert(in *v1.FulcioCert, out *FulcioCert, s apiconversion.Scope) error {
	return autoConvert_v1_FulcioCert_To_v1alpha1_FulcioCert(in, out, s)
}

// Convert_v1alpha1_FulcioStatus_To_v1_FulcioStatus manually converts FulcioStatus.
// v1alpha1 does not have PKCS11 status — it is restored from MarshalData annotations in ConvertTo.
func Convert_v1alpha1_FulcioStatus_To_v1_FulcioStatus(in *FulcioStatus, out *v1.FulcioStatus, s apiconversion.Scope) error {
	return autoConvert_v1alpha1_FulcioStatus_To_v1_FulcioStatus(in, out, s)
}

// Convert_v1_FulcioStatus_To_v1alpha1_FulcioStatus manually converts FulcioStatus from v1 to v1alpha1.
// PKCS11 status is v1-only and intentionally dropped here.
// It is preserved via MarshalData annotation in ConvertFrom.
func Convert_v1_FulcioStatus_To_v1alpha1_FulcioStatus(in *v1.FulcioStatus, out *FulcioStatus, s apiconversion.Scope) error {
	return autoConvert_v1_FulcioStatus_To_v1alpha1_FulcioStatus(in, out, s)
}

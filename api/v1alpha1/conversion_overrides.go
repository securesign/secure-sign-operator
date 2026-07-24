package v1alpha1

import (
	v1 "github.com/securesign/operator/api/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	apiconversion "k8s.io/apimachinery/pkg/conversion"
)

// Manual conversion functions for Spec types where v1 has fields
// that don't exist in v1alpha1 (e.g. ServiceAccountConfig).
// These fields are preserved via MarshalData/UnmarshalData annotation.

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

func Convert_v1alpha1_TrillianService_To_v1_ServiceReference(in *TrillianService, out *v1.ServiceReference, _ apiconversion.Scope) error {
	addressPortToServiceReference(in.Address, in.Port, out)
	return nil
}

func Convert_v1_ServiceReference_To_v1alpha1_TrillianService(in *v1.ServiceReference, out *TrillianService, _ apiconversion.Scope) error {
	serviceReferenceToAddressPort(in, &out.Address, &out.Port)
	return nil
}

func Convert_v1alpha1_CtlogService_To_v1_ServiceReference(in *CtlogService, out *v1.ServiceReference, _ apiconversion.Scope) error {
	addressPortToServiceReference(in.Address, in.Port, out)
	if out.URL != "" && in.Prefix != "" {
		var err error
		if out.URL, err = urlWithPath(out.URL, in.Prefix); err != nil {
			return err
		}
	}
	return nil
}

func Convert_v1_ServiceReference_To_v1alpha1_CtlogService(in *v1.ServiceReference, out *CtlogService, _ apiconversion.Scope) error {
	if in.URL == "" {
		return nil
	}
	base, prefix, err := splitURLPath(in.URL)
	if err != nil {
		return err
	}
	if prefix != "" {
		out.Prefix = prefix
	}
	ref := &v1.ServiceReference{URL: base}
	serviceReferenceToAddressPort(ref, &out.Address, &out.Port)
	return nil
}

func Convert_v1alpha1_FulcioService_To_v1_ServiceRefWithOIDC(in *FulcioService, out *v1.ServiceRefWithOIDC, _ apiconversion.Scope) error {
	addressPortToServiceReference(in.Address, in.Port, &out.ServiceReference)
	return nil
}

func Convert_v1_ServiceRefWithOIDC_To_v1alpha1_FulcioService(in *v1.ServiceRefWithOIDC, out *FulcioService, _ apiconversion.Scope) error {
	serviceReferenceToAddressPort(&in.ServiceReference, &out.Address, &out.Port)
	return nil
}

func Convert_v1alpha1_RekorService_To_v1_ServiceReference(in *RekorService, out *v1.ServiceReference, _ apiconversion.Scope) error {
	addressPortToServiceReference(in.Address, in.Port, out)
	return nil
}

func Convert_v1_ServiceReference_To_v1alpha1_RekorService(in *v1.ServiceReference, out *RekorService, _ apiconversion.Scope) error {
	serviceReferenceToAddressPort(in, &out.Address, &out.Port)
	return nil
}

func Convert_v1alpha1_TsaService_To_v1_ServiceReference(in *TsaService, out *v1.ServiceReference, _ apiconversion.Scope) error {
	addressPortToServiceReference(in.Address, in.Port, out)
	return nil
}

func Convert_v1_ServiceReference_To_v1alpha1_TsaService(in *v1.ServiceReference, out *TsaService, _ apiconversion.Scope) error {
	serviceReferenceToAddressPort(in, &out.Address, &out.Port)
	return nil
}

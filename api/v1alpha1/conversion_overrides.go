package v1alpha1

import (
	"fmt"
	"net/url"
	"regexp"
	"strconv"

	v1 "github.com/securesign/operator/api/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	apiconversion "k8s.io/apimachinery/pkg/conversion"
)

var portRe = regexp.MustCompile(`:(\d+)(?:/|$)`)

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

func Convert_v1alpha1_TrillianService_To_v1_ServiceReference(in *TrillianService, out *v1.ServiceReference, _ apiconversion.Scope) error {
	if in.Address != "" && in.Port != nil {
		out.URL = fmt.Sprintf("%s:%d", in.Address, *in.Port)
	} else if in.Address != "" {
		out.URL = in.Address
	}
	return nil
}

func Convert_v1_ServiceReference_To_v1alpha1_TrillianService(in *v1.ServiceReference, out *TrillianService, _ apiconversion.Scope) error {
	if in.URL == "" {
		return nil
	}
	m := portRe.FindStringSubmatchIndex(in.URL)
	if m == nil {
		out.Address = in.URL
		return nil
	}
	out.Address = in.URL[:m[0]]
	port, err := strconv.ParseInt(in.URL[m[2]:m[3]], 10, 32)
	if err != nil {
		out.Address = in.URL
		return nil
	}
	p := int32(port)
	out.Port = &p
	return nil
}

func Convert_v1alpha1_TrillianService_To_v1_ServiceReference(in *TrillianService, out *v1.ServiceReference, _ apiconversion.Scope) error {
	if in.Address != "" && in.Port != nil {
		out.URL = fmt.Sprintf("%s:%d", in.Address, *in.Port)
	} else if in.Address != "" {
		out.URL = in.Address
	}
	return nil
}

func Convert_v1_ServiceReference_To_v1alpha1_TrillianService(in *v1.ServiceReference, out *TrillianService, _ apiconversion.Scope) error {
	if in.URL == "" {
		return nil
	}
	m := portRe.FindStringSubmatchIndex(in.URL)
	if m == nil {
		out.Address = in.URL
		return nil
	}
	out.Address = in.URL[:m[0]]
	port, err := strconv.ParseInt(in.URL[m[2]:m[3]], 10, 32)
	if err != nil {
		out.Address = in.URL
		return nil
	}
	p := int32(port)
	out.Port = &p
	return nil
}

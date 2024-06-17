package utils

import (
	"github.com/securesign/operator/api/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func CreateConfigMap(instance *v1alpha1.Fulcio, configMapName string) (*corev1.ConfigMap, error) {

	return &corev1.ConfigMap{
		ObjectMeta: v1.ObjectMeta{
			Name:      configMapName,
			Namespace: instance.Namespace,
			Labels:    map[string]string{"config.openshift.io/inject-trusted-cabundle": "true"},
		},
	}, nil
}

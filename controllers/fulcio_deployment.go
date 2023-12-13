package controllers

import (
	"context"

	client "sigs.k8s.io/controller-runtime/pkg/client"

	rhtasv1alpha1 "github.com/securesign/operator/api/v1alpha1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	ctrllog "sigs.k8s.io/controller-runtime/pkg/log"
)

func (r *SecuresignReconciler) ensureFulDeployment(ctx context.Context, m *rhtasv1alpha1.Securesign, namespace string, deploymentName string, sA string, component string, ssapp string) (*appsv1.Deployment, error) {
	log := ctrllog.FromContext(ctx)
	log.Info("ensuring deployment")
	replicas := int32(1)
	mode := int32(0666)
	// Define a new Deployment object
	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      deploymentName,
			Namespace: namespace,
			Labels: map[string]string{
				"app.kubernetes.io/component": component,
				"app.kubernetes.io/name":      ssapp,
				"app.kubernetes.io/instance":  "trusted-artifact-signer",
			},
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app.kubernetes.io/component": component,
					"app.kubernetes.io/name":      ssapp,
					"app.kubernetes.io/instance":  "trusted-artifact-signer",
				},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"app.kubernetes.io/component": component,
						"app.kubernetes.io/name":      ssapp,
						"app.kubernetes.io/instance":  "trusted-artifact-signer",
					},
				},
				Spec: corev1.PodSpec{
					ServiceAccountName: sA,
					Containers: []corev1.Container{
						{
							Name:  ssapp,
							Image: "registry.redhat.io/rhtas-tech-preview/fulcio-rhel9@sha256:0421d44d2da8dd87f05118293787d95686e72c65c0f56dfb9461a61e259b8edc",
							Args: []string{
								"serve",
								"--port=5555",
								"--grpc-port=5554",
								"--ca=fileca",
								"--fileca-key",
								"/var/run/fulcio-secrets/key.pem",
								"--fileca-cert",
								"/var/run/fulcio-secrets/cert.pem",
								"--fileca-key-passwd",
								"$(PASSWORD)",
								"--ct-log-url=http://ctlog.ctlog-system.svc/sigstorescaffolding",
							},
							Env: []corev1.EnvVar{
								{
									Name: "PASSWORD",
									ValueFrom: &corev1.EnvVarSource{
										SecretKeyRef: &corev1.SecretKeySelector{
											Key: "password",
											LocalObjectReference: corev1.LocalObjectReference{
												Name: "fulcio-secret-rh",
											},
										},
									},
								},
							},
							Ports: []corev1.ContainerPort{
								{
									Protocol:      corev1.ProtocolTCP,
									ContainerPort: 5555,
								},
								{
									Protocol:      corev1.ProtocolTCP,
									ContainerPort: 5554,
								},
								{
									Protocol:      corev1.ProtocolTCP,
									ContainerPort: 2112,
								},
							},
							LivenessProbe: &corev1.Probe{
								ProbeHandler: corev1.ProbeHandler{
									HTTPGet: &corev1.HTTPGetAction{
										Path: "/healthz",
										Port: intstr.FromInt(5555),
									},
								},
								InitialDelaySeconds: 10,
							},
							ReadinessProbe: &corev1.Probe{
								ProbeHandler: corev1.ProbeHandler{
									HTTPGet: &corev1.HTTPGetAction{
										Path: "/healthz",
										Port: intstr.FromInt(5555),
									},
								},
								InitialDelaySeconds: 10,
							},
							VolumeMounts: []corev1.VolumeMount{
								{
									Name:      "fulcio-config",
									MountPath: "/etc/fulcio-config",
								},
								{
									Name:      "oidc-info",
									MountPath: "/var/run/fulcio",
								},
								{
									Name:      "fulcio-cert",
									MountPath: "/var/run/fulcio-secrets",
									ReadOnly:  true,
								},
							},
						},
					},
					Volumes: []corev1.Volume{
						{
							Name: "fulcio-config",
							VolumeSource: corev1.VolumeSource{
								ConfigMap: &corev1.ConfigMapVolumeSource{
									LocalObjectReference: corev1.LocalObjectReference{
										Name: "fulcio-server-config",
									},
								},
							},
						},
						{
							Name: "oidc-info",
							VolumeSource: corev1.VolumeSource{
								Projected: &corev1.ProjectedVolumeSource{
									Sources: []corev1.VolumeProjection{
										{
											ConfigMap: &corev1.ConfigMapProjection{
												LocalObjectReference: corev1.LocalObjectReference{
													Name: "kube-root-ca.crt",
												},
												Items: []corev1.KeyToPath{
													{
														Key:  "ca.crt",
														Path: "ca.crt",
														Mode: &mode,
													},
												},
											},
										},
									},
								},
							},
						},
						{
							Name: "fulcio-cert",
							VolumeSource: corev1.VolumeSource{
								Secret: &corev1.SecretVolumeSource{
									SecretName: "fulcio-secret-rh",
									Items: []corev1.KeyToPath{
										{
											Key:  "private",
											Path: "key.pem",
										},
										{
											Key:  "cert",
											Path: "cert.pem",
										},
									},
								},
							},
						},
					},
				},
			},
		},
	}
	// Check if this Deployment already exists else create it in the namespace
	err := r.Get(ctx, client.ObjectKey{Name: deployment.Name, Namespace: namespace}, deployment)
	// If the Deployment doesn't exist, create it but if it does, do nothing
	if err != nil {
		log.Info("Creating a new Deployment")
		err = r.Create(ctx, deployment)
		if err != nil {
			log.Error(err, "Failed to create new Deployment")
			return nil, err
		}
	}
	return deployment, nil
}

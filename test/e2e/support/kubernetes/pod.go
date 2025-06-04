package kubernetes

import (
	"context"
	"io"

	v1 "k8s.io/api/core/v1"
	kubernetes2 "k8s.io/client-go/kubernetes"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
)

func GetPodLogs(ctx context.Context, podName, containerName, ns string) (string, error) {
	cfg := config.GetConfigOrDie()
	var (
		err       error
		clientset *kubernetes2.Clientset
	)
	if clientset, err = kubernetes2.NewForConfig(cfg); err != nil {
		return "", err
	}

	podLogOpts := v1.PodLogOptions{
		Container: containerName,
		Follow:    false,
	}

	req := clientset.CoreV1().Pods(ns).GetLogs(podName, &podLogOpts)
	podLogs, err := req.Stream(ctx)
	if err != nil {
		return "", err
	}
	defer func() {
		_ = podLogs.Close()
	}()
	bodyBytes, err := io.ReadAll(podLogs)
	if err != nil {
		return "", err
	}

	return string(bodyBytes), nil
}

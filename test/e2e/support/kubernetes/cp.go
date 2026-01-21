package kubernetes

import (
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"strings"
	_ "unsafe"

	"github.com/securesign/operator/test/e2e/support"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/kubernetes/scheme"
	v1 "k8s.io/client-go/kubernetes/typed/core/v1"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/remotecommand"
	controllerruntime "sigs.k8s.io/controller-runtime"
)

func CopyToPod(ctx context.Context, config *rest.Config, pod corev1.Pod, srcPath string, destPath string) error {
	client, err := v1.NewForConfig(config)
	if err != nil {
		return err
	}
	reader, writer := io.Pipe()

	go func() {
		defer func() { _ = writer.Close() }()
		_ = support.Tar(srcPath, writer)
	}()

	//remote shell.
	req := client.RESTClient().
		Post().
		Namespace(pod.Namespace).
		Resource("pods").
		Name(pod.Name).
		SubResource("exec").
		VersionedParams(&corev1.PodExecOptions{
			Container: pod.Spec.Containers[0].Name,
			Command:   []string{"tar", "-xf", "-", "-C", destPath},
			Stdin:     true,
			Stdout:    true,
			Stderr:    true,
			TTY:       false,
		}, scheme.ParameterCodec)

	exec, err := remotecommand.NewSPDYExecutor(config, "POST", req.URL())
	if err != nil {
		log.Fatalf("error %s\n", err)
		return err
	}
	err = exec.StreamWithContext(ctx, remotecommand.StreamOptions{
		Stdin:  reader,
		Stdout: os.Stdout,
		Stderr: os.Stderr,
		Tty:    false,
	})
	if err != nil {
		log.Fatalf("error %s\n", err)
		return err
	}
	return nil
}
func CopyFromPod(ctx context.Context, pod corev1.Pod, srcPath string, destPath string) error {
	reader := newRemoteTarPipe(ctx, pod, srcPath)
	return support.Untar(destPath, reader)
}

// inspired by https://github.com/kubernetes/kubectl/blob/master/pkg/cmd/cp/cp.go
type remoteTarPipe struct {
	config     *rest.Config
	client     *v1.CoreV1Client
	srcPath    string
	pod        corev1.Pod
	reader     *io.PipeReader
	outStream  *io.PipeWriter
	bytesRead  uint64
	retries    int
	maxRetries int
	ctx        context.Context
}

func newRemoteTarPipe(ctx context.Context, pod corev1.Pod, srcPath string) *remoteTarPipe {
	t := new(remoteTarPipe)
	t.maxRetries = 30
	t.srcPath = srcPath
	t.pod = pod
	t.config = controllerruntime.GetConfigOrDie()
	t.client = v1.NewForConfigOrDie(t.config)

	t.initReadFrom(0)
	t.ctx = ctx
	return t
}

func (t *remoteTarPipe) initReadFrom(n uint64) {
	t.reader, t.outStream = io.Pipe()
	options := &corev1.PodExecOptions{
		Container: t.pod.Spec.Containers[0].Name,
		Command:   []string{"tar", "cf", "-", "-C", t.srcPath, "."},
		Stdin:     true,
		Stdout:    true,
		Stderr:    true,
		TTY:       false,
	}

	if n > 0 {
		options.Command = []string{"sh", "-c", fmt.Sprintf("%s | tail -c+%d", strings.Join(options.Command, " "), n)}
	}

	req := t.client.RESTClient().
		Post().
		Namespace(t.pod.Namespace).
		Resource("pods").
		Name(t.pod.Name).
		SubResource("exec").
		VersionedParams(options, scheme.ParameterCodec)

	exec, err := remotecommand.NewSPDYExecutor(t.config, "POST", req.URL())
	if err != nil {
		log.Fatalf("error %s\n", err)
	}

	go func() {
		defer func() { _ = t.outStream.Close() }()
		_ = exec.StreamWithContext(t.ctx, remotecommand.StreamOptions{
			Stdin:  os.Stdin,
			Stdout: t.outStream,
			Stderr: os.Stderr,
			Tty:    false,
		})
	}()
}

func (t *remoteTarPipe) Read(p []byte) (n int, err error) {
	n, err = t.reader.Read(p)
	if err != nil {
		if t.retries < t.maxRetries {
			t.retries++
			fmt.Printf("Resuming copy at %d bytes, retry %d/%d\n", t.bytesRead, t.retries, t.maxRetries)
			t.initReadFrom(t.bytesRead + 1)
			err = nil
		} else {
			fmt.Printf("Dropping out copy after %d retries\n", t.retries)
		}
	} else {
		t.bytesRead += uint64(n)
	}
	return
}

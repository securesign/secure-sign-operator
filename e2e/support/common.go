package support

import (
	"context"
	"io"
	"os"
	"time"

	"github.com/docker/docker/api/types"
	docker "github.com/docker/docker/client"
	"github.com/google/uuid"
	"github.com/onsi/ginkgo/v2/dsl/core"
	. "github.com/onsi/gomega"
	"github.com/securesign/operator/api/v1alpha1"
	"github.com/securesign/operator/client"
	"gopkg.in/yaml.v2"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/json"
	client2 "sigs.k8s.io/controller-runtime/pkg/client"
)

const fromImage = "alpine:latest"

func CreateTestNamespace(ctx context.Context, cli client.Client) *v1.Namespace {
	ns := &v1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-" + uuid.New().String(),
		},
	}
	Expect(cli.Create(ctx, ns)).To(Succeed())
	core.GinkgoWriter.Println("Created test namespace: " + ns.Name)
	return ns
}

func PrepareImage(ctx context.Context, targetImageName string) {
	dockerCli, err := docker.NewClientWithOpts(docker.FromEnv, docker.WithAPIVersionNegotiation())
	Expect(err).ToNot(HaveOccurred())

	var pull io.ReadCloser
	pull, err = dockerCli.ImagePull(ctx, fromImage, types.ImagePullOptions{})
	Expect(err).ToNot(HaveOccurred())
	_, err = io.Copy(core.GinkgoWriter, pull)
	Expect(err).ToNot(HaveOccurred())
	defer pull.Close()

	Expect(dockerCli.ImageTag(ctx, fromImage, targetImageName)).To(Succeed())
	var push io.ReadCloser
	push, err = dockerCli.ImagePush(ctx, targetImageName, types.ImagePushOptions{RegistryAuth: types.RegistryAuthFromSpec})
	Expect(err).ToNot(HaveOccurred())
	_, err = io.Copy(core.GinkgoWriter, push)
	Expect(err).ToNot(HaveOccurred())
	defer push.Close()
	// wait for a while to be sure that the image landed in the registry
	time.Sleep(10 * time.Second)
}

func EnvOrDefault(env string, def string) string {
	val, ok := os.LookupEnv(env)
	if ok {
		return val
	}
	return def
}

func DumpNamespace(ctx context.Context, cli client.Client, ns string) {

	core.GinkgoWriter.Println("----------------------- Dumping namespace " + ns + " -----------------------")
	fulcios := &v1alpha1.FulcioList{}
	cli.List(ctx, fulcios, client2.InNamespace(ns))
	core.GinkgoWriter.Println("Fulcios:")
	for _, p := range fulcios.Items {
		core.GinkgoWriter.Println(toYAMLNoManagedFields(&p))
	}

	rekors := &v1alpha1.RekorList{}
	cli.List(ctx, rekors, client2.InNamespace(ns))
	core.GinkgoWriter.Println("Rekors:")
	for _, p := range rekors.Items {
		core.GinkgoWriter.Println(toYAMLNoManagedFields(&p))
	}

	tufs := &v1alpha1.TufList{}
	cli.List(ctx, tufs, client2.InNamespace(ns))
	core.GinkgoWriter.Println("Tufs:")
	for _, p := range tufs.Items {
		core.GinkgoWriter.Println(toYAMLNoManagedFields(&p))
	}

	ctlogs := &v1alpha1.CTlogList{}
	cli.List(ctx, ctlogs, client2.InNamespace(ns))
	core.GinkgoWriter.Println("CTLogs:")
	for _, p := range ctlogs.Items {
		core.GinkgoWriter.Println(toYAMLNoManagedFields(&p))
	}

	trillians := &v1alpha1.TrillianList{}
	cli.List(ctx, trillians, client2.InNamespace(ns))
	core.GinkgoWriter.Println("Trillians:")
	for _, p := range trillians.Items {
		core.GinkgoWriter.Println(toYAMLNoManagedFields(&p))
	}

	pods, _ := cli.CoreV1().Pods(ns).List(ctx, metav1.ListOptions{})
	core.GinkgoWriter.Println("Pods:")
	for _, p := range pods.Items {
		core.GinkgoWriter.Println(toYAMLNoManagedFields(&p))
	}

	secrets, _ := cli.CoreV1().Secrets(ns).List(ctx, metav1.ListOptions{})
	core.GinkgoWriter.Println("Secrets:")
	for _, p := range secrets.Items {
		core.GinkgoWriter.Println(toYAMLNoManagedFields(&p))
	}

	cm, _ := cli.CoreV1().Secrets(ns).List(ctx, metav1.ListOptions{})
	core.GinkgoWriter.Println("ConfigMaps:")
	for _, p := range cm.Items {
		core.GinkgoWriter.Println(toYAMLNoManagedFields(&p))
	}
}

func toYAMLNoManagedFields(value runtime.Object) string {
	object, _ := json.Marshal(value)

	mapdata := map[string]interface{}{}
	json.Unmarshal(object, &mapdata)

	if m, ok := mapdata["metadata"].(map[string]interface{}); ok {
		delete(m, "managedFields")
	}
	out, _ := yaml.Marshal(mapdata)

	return string(out)
}

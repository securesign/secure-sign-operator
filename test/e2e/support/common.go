package support

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/asn1"
	"encoding/pem"
	"fmt"
	"log"
	"math/big"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"time"

	routev1 "github.com/openshift/api/route/v1"
	monitoringv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	v12 "k8s.io/api/apps/v1"
	v13 "k8s.io/api/batch/v1"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client/config"

	"github.com/google/go-containerregistry/pkg/name"
	"github.com/google/go-containerregistry/pkg/v1/random"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/google/uuid"
	"github.com/onsi/ginkgo/v2"
	"github.com/onsi/ginkgo/v2/dsl/core"
	. "github.com/onsi/gomega"
	olm "github.com/operator-framework/api/pkg/operators/v1"
	olmAlpha "github.com/operator-framework/api/pkg/operators/v1alpha1"
	"github.com/securesign/operator/api/v1alpha1"
	rhtasv1alpha1 "github.com/securesign/operator/api/v1alpha1"
	tsaUtils "github.com/securesign/operator/internal/controller/tsa/utils"
	"gopkg.in/yaml.v2"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/json"
	"sigs.k8s.io/controller-runtime/pkg/client"
	runtimeCli "sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	CertPassword = "LetMeIn123"
)

func IsCIEnvironment() bool {
	if val, present := os.LookupEnv("CI"); present {
		b, _ := strconv.ParseBool(val)
		return b
	}
	return false
}

func CreateClient() (runtimeCli.Client, error) {
	scheme := runtime.NewScheme()
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(monitoringv1.AddToScheme(scheme))
	utilruntime.Must(rhtasv1alpha1.AddToScheme(scheme))
	utilruntime.Must(routev1.AddToScheme(scheme))
	utilruntime.Must(olmAlpha.AddToScheme(scheme))
	utilruntime.Must(olm.AddToScheme(scheme))

	cfg, err := config.GetConfig()
	if err != nil {
		return nil, err
	}

	return runtimeCli.New(cfg, runtimeCli.Options{Scheme: scheme})

}
func CreateTestNamespace(ctx context.Context, cli client.Client) *v1.Namespace {
	sp := ginkgo.CurrentSpecReport()
	fn := filepath.Base(sp.LeafNodeLocation.FileName)
	// Replace invalid characters with '-'
	re := regexp.MustCompile("[^a-z0-9-]")
	name := re.ReplaceAllString(strings.TrimSuffix(fn, filepath.Ext(fn)), "-")

	ns := &v1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: name + "-",
		},
	}
	Expect(cli.Create(ctx, ns)).To(Succeed())
	core.GinkgoWriter.Println("Created test namespace: " + ns.Name)
	return ns
}

func PrepareImage(ctx context.Context) string {
	if v, ok := os.LookupEnv("TEST_IMAGE"); ok {
		return v
	}

	image, err := random.Image(1024, 8)
	if err != nil {
		panic(err.Error())
	}

	targetImageName := fmt.Sprintf("ttl.sh/%s:15m", uuid.New().String())
	ref, err := name.ParseReference(targetImageName)
	if err != nil {
		panic(err.Error())
	}

	pusher, err := remote.NewPusher()
	if err != nil {
		panic(err.Error())
	}

	err = pusher.Push(ctx, ref, image)
	if err != nil {
		panic(err.Error())
	}
	return targetImageName
}

func EnvOrDefault(env string, def string) string {
	val, ok := os.LookupEnv(env)
	if ok {
		return val
	}
	return def
}

func DumpNamespace(ctx context.Context, cli client.Client, ns string) {

	// Example usage with mock data
	k8s := map[string]logTarget{}

	toDump := map[string]client.ObjectList{
		"securesign.yaml": &v1alpha1.SecuresignList{},
		"fulcio.yaml":     &v1alpha1.FulcioList{},
		"rekor.yaml":      &v1alpha1.RekorList{},
		"tuf.yaml":        &v1alpha1.TufList{},
		"ctlog.yaml":      &v1alpha1.CTlogList{},
		"trillian.yaml":   &v1alpha1.TrillianList{},
		"tsa.yaml":        &v1alpha1.TimestampAuthorityList{},
		"pod.yaml":        &v1.PodList{},
		"configmap.yaml":  &v1.ConfigMapList{},
		"deployment.yaml": &v12.DeploymentList{},
		"job.yaml":        &v13.JobList{},
		"cronjob.yaml":    &v13.CronJobList{},
		"event.yaml":      &v1.EventList{},
	}

	core.GinkgoWriter.Println("----------------------- Dumping namespace " + ns + " -----------------------")

	for key, obj := range toDump {
		if dump, err := dumpK8sObjects(ctx, cli, obj, ns); err == nil {
			k8s[key] = logTarget{
				reader: strings.NewReader(dump),
				size:   int64(len(dump)),
			}
		} else {
			log.Println(fmt.Errorf("dump failed for %s: %w", key, err))
		}
	}

	// Create the output file
	fileName := "k8s-dump-" + ns + ".tar.gz"
	outFile, err := os.Create(fileName)
	if err != nil {
		log.Fatalf("Failed to create %s file: %v", fileName, err)
	}

	if err := createArchive(outFile, k8s); err != nil {
		log.Fatalf("Failed to create %s: %v", fileName, err)
	}
}

func dumpK8sObjects(ctx context.Context, cli client.Client, list client.ObjectList, namespace string) (string, error) {
	var builder strings.Builder

	if err := cli.List(ctx, list, client.InNamespace(namespace)); err != nil {
		return "", err
	}

	// Use reflection to access the Items field
	items := reflect.ValueOf(list).Elem().FieldByName("Items")

	// Check if Items field is valid and is a slice
	if !items.IsValid() || items.Kind() != reflect.Slice {
		return "", fmt.Errorf("invalid items field in list: %v", items)
	}

	// Iterate over the items slice
	for i := 0; i < items.Len(); i++ {
		item := items.Index(i).Addr().Interface().(client.Object)
		yamlData := toYAMLNoManagedFields(item)
		builder.WriteString("\n---\n")
		builder.WriteString(yamlData)
	}
	return builder.String(), nil
}

func toYAMLNoManagedFields(value runtime.Object) string {
	object, _ := json.Marshal(value)

	mapdata := map[string]interface{}{}
	_ = json.Unmarshal(object, &mapdata)

	if m, ok := mapdata["metadata"].(map[string]interface{}); ok {
		delete(m, "managedFields")
	}
	out, _ := yaml.Marshal(mapdata)

	return fmt.Sprintf("%s\n", out)
}

func InitFulcioSecret(ns string, name string) *v1.Secret {
	public, private, root, err := InitCertificates(true)
	if err != nil {
		return nil
	}
	return &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: ns,
		},
		Data: map[string][]byte{
			"password": []byte(CertPassword),
			"private":  private,
			"public":   public,
			"cert":     root,
		},
	}
}

func InitRekorSecret(ns string, name string) *v1.Secret {
	public, private, _, err := InitCertificates(false)
	if err != nil {
		return nil
	}
	return &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: ns,
		},
		Data: map[string][]byte{
			"private": private,
			"public":  public,
		},
	}
}

func InitCTSecret(ns string, name string) *v1.Secret {
	public, private, _, err := InitCertificates(false)
	if err != nil {
		return nil
	}
	return &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: ns,
		},
		Data: map[string][]byte{
			"private": private,
			"public":  public,
		},
	}
}

func InitCertificates(passwordProtected bool) ([]byte, []byte, []byte, error) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, nil, nil, err
	}

	// private
	privateKeyBytes, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		return nil, nil, nil, err
	}
	var block *pem.Block
	if passwordProtected {
		block, err = x509.EncryptPEMBlock(rand.Reader, "EC PRIVATE KEY", privateKeyBytes, []byte(CertPassword), x509.PEMCipher3DES) //nolint:staticcheck
		if err != nil {
			return nil, nil, nil, err
		}
	} else {
		block = &pem.Block{
			Type:  "EC PRIVATE KEY",
			Bytes: privateKeyBytes,
		}
	}
	privateKeyPem := pem.EncodeToMemory(block)

	// public key
	publicKeyBytes, err := x509.MarshalPKIXPublicKey(&key.PublicKey)
	if err != nil {
		return nil, nil, nil, err
	}
	publicKeyPem := pem.EncodeToMemory(
		&pem.Block{
			Type:  "PUBLIC KEY",
			Bytes: publicKeyBytes,
		},
	)

	notBefore := time.Now()
	notAfter := notBefore.Add(365 * 24 * 10 * time.Hour)

	issuer := pkix.Name{
		CommonName:         "local",
		Country:            []string{"CR"},
		Organization:       []string{"RedHat"},
		Province:           []string{"Czech Republic"},
		Locality:           []string{"Brno"},
		OrganizationalUnit: []string{"QE"},
	}
	//Create certificate templet
	template := x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               issuer,
		SignatureAlgorithm:    x509.ECDSAWithSHA256,
		NotBefore:             notBefore,
		NotAfter:              notAfter,
		BasicConstraintsValid: true,
		IsCA:                  true,
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
		Issuer:                issuer,
	}
	//Create certificate using templet
	derBytes, err := x509.CreateCertificate(rand.Reader, &template, &template, &key.PublicKey, key)
	if err != nil {
		return nil, nil, nil, err

	}
	//pem encoding of certificate
	root := pem.EncodeToMemory(
		&pem.Block{
			Type:  "CERTIFICATE",
			Bytes: derBytes,
		},
	)
	return publicKeyPem, privateKeyPem, root, err
}

func InitTsaSecrets(ns string, name string) *v1.Secret {
	config := &tsaUtils.TsaCertChainConfig{
		RootPrivateKeyPassword:          []byte(CertPassword),
		IntermediatePrivateKeyPasswords: [][]byte{[]byte(CertPassword)},
		LeafPrivateKeyPassword:          []byte(CertPassword),
	}
	_, rootPrivateKey, rootCA, err := InitCertificates(true)
	if err != nil {
		return nil
	}
	config.RootPrivateKey = rootPrivateKey

	intermediatePublicKey, intermediatePrivateKey, _, err := InitCertificates(true)
	if err != nil {
		return nil
	}
	config.IntermediatePrivateKeys = append(config.IntermediatePrivateKeys, intermediatePrivateKey)

	leafPublicKey, leafPrivateKey, _, err := InitCertificates(true)
	if err != nil {
		return nil
	}
	config.LeafPrivateKey = leafPrivateKey

	notBefore := time.Now()
	notAfter := notBefore.Add(365 * 24 * 10 * time.Hour)
	oidExtendedKeyUsage := asn1.ObjectIdentifier{2, 5, 29, 37}
	oidTimeStamping := asn1.ObjectIdentifier{1, 3, 6, 1, 5, 5, 7, 3, 8}
	ekuValues, err := asn1.Marshal([]asn1.ObjectIdentifier{oidTimeStamping})
	if err != nil {
		return nil
	}

	ekuExtension := pkix.Extension{
		Id:       oidExtendedKeyUsage,
		Critical: true,
		Value:    ekuValues,
	}

	issuer := pkix.Name{
		CommonName:         "local",
		Country:            []string{"CR"},
		Organization:       []string{"RedHat"},
		Province:           []string{"Czech Republic"},
		Locality:           []string{"Brno"},
		OrganizationalUnit: []string{"QE"},
	}

	certTemplate := x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               issuer,
		BasicConstraintsValid: true,
		IsCA:                  true,
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageTimeStamping},
		ExtraExtensions:       []pkix.Extension{ekuExtension},
		Issuer:                issuer,
		NotBefore:             notBefore,
		NotAfter:              notAfter,
	}

	block, _ := pem.Decode(rootPrivateKey)
	keyBytes := block.Bytes
	if x509.IsEncryptedPEMBlock(block) { //nolint:staticcheck
		keyBytes, err = x509.DecryptPEMBlock(block, []byte(CertPassword)) //nolint:staticcheck
		if err != nil {
			return nil
		}
	}

	rootPrivKey, err := x509.ParseECPrivateKey(keyBytes)
	if err != nil {
		return nil
	}

	block, _ = pem.Decode(intermediatePublicKey)
	keyBytes = block.Bytes
	interPubKey, err := x509.ParsePKIXPublicKey(keyBytes)
	if err != nil {
		return nil
	}

	block, _ = pem.Decode(rootCA)
	rootCert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return nil
	}

	intermediateCert, err := x509.CreateCertificate(rand.Reader, &certTemplate, rootCert, interPubKey, rootPrivKey)
	if err != nil {
		return nil
	}

	block, _ = pem.Decode(leafPublicKey)
	keyBytes = block.Bytes
	leafPuKey, err := x509.ParsePKIXPublicKey(keyBytes)
	if err != nil {
		return nil
	}

	leafCert, err := x509.CreateCertificate(rand.Reader, &certTemplate, rootCert, leafPuKey, rootPrivKey)
	if err != nil {
		return nil
	}

	intermediatePEM := pem.EncodeToMemory(&pem.Block{
		Type:  "CERTIFICATE",
		Bytes: intermediateCert,
	})

	leafPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "CERTIFICATE",
		Bytes: leafCert,
	})
	certificateChain := append(leafPEM, intermediatePEM...)
	certificateChain = append(certificateChain, rootCA...)
	config.CertificateChain = certificateChain

	return &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: ns,
		},
		Data: map[string][]byte{
			"rootPrivateKey":            config.RootPrivateKey,
			"rootPrivateKeyPassword":    config.RootPrivateKeyPassword,
			"interPrivateKey-0":         config.IntermediatePrivateKeys[0],
			"interPrivateKeyPassword-0": config.IntermediatePrivateKeyPasswords[0],
			"leafPrivateKey":            config.LeafPrivateKey,
			"leafPrivateKeyPassword":    config.LeafPrivateKeyPassword,
			"certificateChain":          config.CertificateChain,
		},
	}
}

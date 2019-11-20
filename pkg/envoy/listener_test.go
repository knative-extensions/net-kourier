package envoy

import (
	"kourier/pkg/config"
	"os"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"k8s.io/client-go/kubernetes/fake"

	core "github.com/envoyproxy/go-control-plane/envoy/api/v2/core"
	route "github.com/envoyproxy/go-control-plane/envoy/api/v2/route"
	"gotest.tools/assert"
	is "gotest.tools/assert/cmp"
	v1 "k8s.io/api/core/v1"
)

func TestCreateHTTPListener(t *testing.T) {
	manager := newHttpConnectionManager([]*route.VirtualHost{})

	kubeClient := fake.NewSimpleClientset()

	l, err := newExternalEnvoyListener(false, &manager, kubeClient)
	if err != nil {
		t.Error(err)
	}

	assert.Equal(t, core.SocketAddress_TCP, l.Address.GetSocketAddress().Protocol)
	assert.Equal(t, "0.0.0.0", l.Address.GetSocketAddress().Address)
	assert.Equal(t, config.HttpPortExternal, l.Address.GetSocketAddress().GetPortValue())
	assert.Assert(t, is.Nil(l.FilterChains[0].TlsContext)) //TLS not configured
}

func TestCreateHTTPSListener(t *testing.T) {
	certsSecretNamespace := "default"
	certsSecretName := "kourier-certs"
	err := setHTTPSEnvs(certsSecretNamespace, certsSecretName)
	if err != nil {
		t.Error(err)
	}

	// Create a Kubernetes Client with the Secret needed
	cert := "some_cert"
	key := "some_key"
	kubeClient := fake.NewSimpleClientset()
	secret := newSecret(certsSecretName, cert, key)
	_, err = kubeClient.CoreV1().Secrets(certsSecretNamespace).Create(&secret)
	if err != nil {
		t.Error(err)
	}

	manager := newHttpConnectionManager([]*route.VirtualHost{})

	l, err := newExternalEnvoyListener(true, &manager, kubeClient)
	if err != nil {
		t.Error(err)
	}

	assert.Equal(t, core.SocketAddress_TCP, l.Address.GetSocketAddress().Protocol)
	assert.Equal(t, "0.0.0.0", l.Address.GetSocketAddress().Address)
	assert.Equal(t, config.HttpsPortExternal, l.Address.GetSocketAddress().GetPortValue())

	// Check that TLS is configured
	certs := l.FilterChains[0].TlsContext.CommonTlsContext.TlsCertificates[0]
	assert.Equal(t, cert, string(certs.CertificateChain.GetInlineBytes()))
	assert.Equal(t, key, string(certs.PrivateKey.GetInlineBytes()))
}

func newSecret(name string, cert string, key string) v1.Secret {
	return v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
		Data: map[string][]byte{
			"tls.crt": []byte(cert),
			"tls.key": []byte(key),
		},
	}
}

func setHTTPSEnvs(secretNamespace string, secretName string) error {
	err := os.Setenv("CERTS_SECRET_NAMESPACE", secretNamespace)
	if err != nil {
		return err
	}

	err = os.Setenv("CERTS_SECRET_NAME", secretName)
	if err != nil {
		return err
	}

	return nil
}

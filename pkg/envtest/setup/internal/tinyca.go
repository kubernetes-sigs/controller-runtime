package internal

// NB(directxman12): nothing has verified that this has good settings.  In fact,
// the setting generated here are probably terrible, but they're fine for integration
// tests.  These ABSOLUTELY SHOULD NOT ever be exposed in the public API.  They're
// ONLY for use with envtest's ability to configure webhook testing.
// If I didn't otherwise not want to add a dependency on cfssl, I'd just use that.

import (
	"fmt"
	"crypto/x509"
	"crypto/x509/pkix"
	"crypto"
	"crypto/rsa"
	"time"
	"math/big"
	crand "crypto/rand"
	"encoding/pem"
	"os"
	"io/ioutil"
	"path/filepath"
	"net"

	certutil "k8s.io/client-go/util/cert"
	"k8s.io/client-go/rest"
)

var (
	rsaKeySize = 2048 // a decent number, as of 2019
	bigOne = big.NewInt(1)
)

// CertPair is a private key and certificate for use for client auth, as a CA, or serving.
type CertPair struct {
	Key crypto.Signer
	Cert *x509.Certificate
}

// CertData returns the PEM-encoded version of the certificate for this pair.
func (k CertPair) CertData() []byte {
	return pem.EncodeToMemory(&pem.Block{
		Type: "CERTIFICATE",
		Bytes: k.Cert.Raw,
	})
}

// Data encodes keypair in the appropriate formats for on-disk storage (PEM and
// PKCS8, respectively).
func (k CertPair) Data() (cert []byte, key []byte, err error) {
	cert = pem.EncodeToMemory(&pem.Block{
		Type: "CERTIFICATE",
		Bytes: k.Cert.Raw,
	})

	rawKeyData, err := x509.MarshalPKCS8PrivateKey(k.Key)
	if err != nil {
		return nil, nil, fmt.Errorf("unable to encode private key: %v", err)
	}

	key = pem.EncodeToMemory(&pem.Block{
		Type: "PRIVATE KEY",
		Bytes: rawKeyData,
	})

	return cert, key, nil
}

// TinyCA supports signing serving certs and client-certs,
// and can be used as an auth mechanism with envtest.
type TinyCA struct {
	CA CertPair
	orgName string

	nextSerial *big.Int

	BaseDir string
	caDir string
}

// newPrivateKey generates a new private key of a relatively sane size (see
// rsaKeySize).
func newPrivateKey() (crypto.Signer, error) {
	return rsa.GenerateKey(crand.Reader, rsaKeySize)
}

func NewTinyCA() (*TinyCA, error) {
	caPrivateKey, err := newPrivateKey()
	if err != nil {
		return nil, fmt.Errorf("unable to generate private key for CA: %v", err)
	}
	caCfg := certutil.Config{CommonName: "envtest-environment", Organization: []string{"envtest"}}
	caCert, err := certutil.NewSelfSignedCACert(caCfg, caPrivateKey)
	if err != nil {
		return nil, fmt.Errorf("unable to generate certificate for CA: %v", err)
	}

	return &TinyCA{
		CA: CertPair{Key: caPrivateKey, Cert: caCert},
		orgName: "envtest",
		nextSerial: big.NewInt(1),
	}, nil
}

func (c *TinyCA) makeCert(cfg certutil.Config) (CertPair, error) {
	now := time.Now()

	key, err := newPrivateKey()
	if err != nil {
		return CertPair{}, fmt.Errorf("unable to create private key: %v", err)
	}

	serial := new(big.Int).Set(c.nextSerial)
	c.nextSerial.Add(c.nextSerial, bigOne)

	template := x509.Certificate{
		Subject: pkix.Name{CommonName: cfg.CommonName, Organization: cfg.Organization},
		DNSNames: cfg.AltNames.DNSNames,
		IPAddresses: cfg.AltNames.IPs,
		SerialNumber: serial,

		KeyUsage: x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage: cfg.Usages,

		// technically not necessary for testing, but let's set anyway just in case.
		NotBefore: now.UTC(),
		// 1 week -- the default for cfssl, and just long enough for a
		// long-term test, but not too long that anyone would try to use this
		// seriously.
		NotAfter: now.Add(168*time.Hour).UTC(),
	}

	certRaw, err := x509.CreateCertificate(crand.Reader, &template, c.CA.Cert, key.Public(), c.CA.Key)
	if err != nil {
		return CertPair{}, fmt.Errorf("unable to create certificate: %v", err)
	}

	cert, err := x509.ParseCertificate(certRaw)
	if err != nil {
		return CertPair{}, fmt.Errorf("generated invalid certificate, could not parse: %v", err)
	}
	
	return CertPair{
		Key: key,
		Cert: cert,
	}, nil
}

// NewServingCert returns a new CertPair for a serving HTTPS on localhost.
func (c *TinyCA) NewServingCert() (CertPair, error) {
	return c.makeCert(certutil.Config{
		CommonName: "localhost",
		Organization: []string{c.orgName},
		AltNames: certutil.AltNames{
			DNSNames: []string{"localhost"},
			IPs: []net.IP{net.IPv4(127, 0, 0, 1)},
		},
		Usages: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	})
}

func (c *TinyCA) NewUser(info User) (CertPair, error) {
	return c.makeCert(certutil.Config{
		CommonName: info.Username,
		Organization: info.Groups,
		Usages: []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
	})
}

func (c *TinyCA) RegisterUser(info User, cfg *rest.Config) error {
	certs, err := c.NewUser(info)
	if err != nil {
		return err
	}

	certData, keyData, err := certs.Data()
	if err != nil {
		return fmt.Errorf("unable to encode certificate data: %v", err)
	}

	cfg.CertData = certData
	cfg.KeyData = keyData

	// Don't set the CA data -- it needs to be the API server's serving CA, not
	// this client CA

	return nil
}

func (c *TinyCA) Start() ([]string, error) {
	caCert := c.CA.CertData()

	var err error
	c.caDir, err = ioutil.TempDir(c.BaseDir, "envtest-ca-certs-")
	if err != nil {
		return nil, fmt.Errorf("unable to create directory for CA certificates: %v", err)
	}
	defer func() {
		// clean up in case of error
		if err != nil {
			os.RemoveAll(c.caDir)
			c.caDir = ""
			panic(err)
		}
	}()

	// NB(directxman12): re-use err so that we auto clean up

	caCertPath := filepath.Join(c.caDir, "client-ca.crt")
	err = ioutil.WriteFile(caCertPath, caCert, 0640)
	if err != nil {
		return nil, fmt.Errorf("unable to write CA cert: %v", err)
	}

	return []string{"--client-ca-file="+caCertPath}, nil
}

func (c *TinyCA) Stop() error {
	if c.caDir == "" {
		return nil
	}
	if err := os.RemoveAll(c.caDir); err != nil {
		return fmt.Errorf("unable to remove temporary CA cert directory: %v", err)
	}
	return nil
}

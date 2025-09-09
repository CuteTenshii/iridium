package cli

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"net/http"
	"os"
	"strings"
	"time"

	"golang.org/x/crypto/acme/autocert"
)

func GenerateSelfSignedCert(host string) (certPEM []byte, keyPEM []byte, err error) {
	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to generate private key: %v", err)
	}

	certPath := fmt.Sprintf("%s.crt", host)
	keyPath := fmt.Sprintf("%s.key", host)
	template := x509.Certificate{
		SerialNumber:          big.NewInt(time.Now().UnixNano()),
		Subject:               pkix.Name{CommonName: host},
		NotBefore:             time.Now(),
		NotAfter:              time.Now().Add(365 * 24 * time.Hour),
		KeyUsage:              x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		DNSNames:              []string{host},
	}

	derBytes, err := x509.CreateCertificate(rand.Reader, &template, &template, &priv.PublicKey, priv)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create certificate: %v", err)
	}

	certOut, err := os.Create(certPath)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to open cert.pem for writing: %v", err)
	}
	pem.Encode(certOut, &pem.Block{Type: "CERTIFICATE", Bytes: derBytes})
	certOut.Close()

	keyOut, err := os.Create(keyPath)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to open key.pem for writing: %v", err)
	}
	b, _ := x509.MarshalECPrivateKey(priv)
	pem.Encode(keyOut, &pem.Block{Type: "EC PRIVATE KEY", Bytes: b})
	keyOut.Close()

	fmt.Printf("✅ Generated self-signed cert: %s and %s\n", certPath, keyPath)
	return certPEM, keyPEM, nil
}

func GenerateACMECert(domain string) (certPEM []byte, keyPEM []byte, err error) {
	m := &autocert.Manager{
		Prompt:     autocert.AcceptTOS,
		Cache:      autocert.DirCache("certs"),
		HostPolicy: autocert.HostWhitelist(domain),
	}

	// Start a temporary HTTP server to handle ACME challenges
	challengeHandler := m.HTTPHandler(nil)
	http.HandleFunc("/.well-known/acme-challenge/", func(w http.ResponseWriter, r *http.Request) {
		token := strings.TrimPrefix(r.URL.Path, "/.well-known/acme-challenge/")
		fmt.Printf("[ACME] Challenge requested: token=%s", token)
		challengeHandler.ServeHTTP(w, r)
	})
	err = http.ListenAndServe(":80", nil)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to start HTTP server for ACME challenge: %v", err)
	}
	fmt.Println("Started HTTP server on port 80 for ACME challenge.\nIf you are running this behind Docker, ensure port 80 is exposed.\nIf you are using a firewall, ensure port 80 is open.")

	cert, err := m.GetCertificate(&tls.ClientHelloInfo{ServerName: domain})
	if err != nil {
		return nil, nil, fmt.Errorf("failed to obtain certificate: %v", err)
	}

	certFile := domain + ".crt"
	keyFile := domain + ".key"
	os.WriteFile(certFile, cert.Certificate[0], 0644)
	os.WriteFile(keyFile, cert.PrivateKey.(*ecdsa.PrivateKey).D.Bytes(), 0600)

	fmt.Printf("✅ Obtained TLS certificate: %s and %s\n", certFile, keyFile)
	return
}

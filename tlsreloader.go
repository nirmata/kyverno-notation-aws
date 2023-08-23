package main

import (
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"fmt"

	"github.com/go-logr/logr"
	tlsMgr "github.com/kyverno/pkg/tls"
	"github.com/pkg/errors"
)

type TLSReloader interface {
	GetCertificate(*tls.ClientHelloInfo) (*tls.Certificate, error)
}

type tlsreloader struct {
	logger      logr.Logger
	tlsinformer *chan tlsMgr.TLSCerts
	cachedcert  *tls.Certificate
}

func NewTLSReloader(logger logr.Logger, tlsinformer *chan tlsMgr.TLSCerts) (TLSReloader, error) {
	tlsCerts := <-*tlsinformer
	logger.Info(fmt.Sprintf("Writing tls certs to certs/ folder cert=%v", string(certificateToPem(tlsCerts.Cert))))

	cert, err := tls.X509KeyPair(certificateToPem(tlsCerts.Cert), privateKeyToPem(tlsCerts.Key))
	if err != nil {
		return nil, err
	}

	logger.Info("TLS reloader created")
	return &tlsreloader{
		logger:      logger,
		tlsinformer: tlsinformer,
		cachedcert:  &cert,
	}, nil
}

func (t *tlsreloader) GetCertificate(*tls.ClientHelloInfo) (*tls.Certificate, error) {
	t.logger.Info("fetching certificates from getCertificate function")
	select {
	case certs := <-*t.tlsinformer:
		cert, err := tls.X509KeyPair(certificateToPem(certs.Cert), privateKeyToPem(certs.Key))
		if err != nil {
			return nil, err
		}
		t.cachedcert = &cert

		t.logger.V(2).Info("TLS certs renewed")
	default:
		t.logger.V(2).Info("TLS certs not renewed")
	}

	if t.cachedcert == nil {
		return nil, errors.Errorf("TLS certs not found")
	}
	return t.cachedcert, nil
}

func privateKeyToPem(rsaKey *rsa.PrivateKey) []byte {
	privateKey := &pem.Block{
		Type:  "PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(rsaKey),
	}
	return pem.EncodeToMemory(privateKey)
}

func certificateToPem(certs ...*x509.Certificate) []byte {
	var raw []byte
	for _, cert := range certs {
		certificate := &pem.Block{
			Type:  "CERTIFICATE",
			Bytes: cert.Raw,
		}
		raw = append(raw, pem.EncodeToMemory(certificate)...)
	}
	return raw
}

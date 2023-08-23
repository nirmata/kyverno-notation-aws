package main

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"fmt"

	"github.com/go-logr/logr"
	knvTypes "github.com/nirmata/kyverno-notation-verifier/types"
	"github.com/pkg/errors"
	tlsMgr "github.com/vishal-chdhry/kyverno-pkg/tls"
	corev1 "k8s.io/api/core/v1"
	corelistersv1 "k8s.io/client-go/listers/core/v1"
)

type TLSReloader interface {
	GetCertificate(*tls.ClientHelloInfo) (*tls.Certificate, error)
}

type tlsreloader struct {
	logger          logr.Logger
	tlsinformer     *chan tlsMgr.TLSCerts
	tlsSecretLister corelistersv1.SecretLister
	tlsMgrConfig    *tlsMgr.Config
	cachedcert      *tls.Certificate
}

func NewTLSReloader(logger logr.Logger, tlsinformer *chan tlsMgr.TLSCerts, tlsSecretLister corelistersv1.SecretLister, tlsMgrConfig *tlsMgr.Config) (TLSReloader, error) {
	tlsCerts := <-*tlsinformer
	logger.Info(fmt.Sprintf("Writing tls certs to certs/ folder cert=%v key=%v", tlsCerts.Cert, tlsCerts.Key))

	cert, err := fetchTLSCertsFromSecret(tlsSecretLister, tlsMgrConfig)
	if err != nil {
		return nil, err
	}

	logger.Info("TLS reloader created")
	return &tlsreloader{
		logger:          logger,
		tlsinformer:     tlsinformer,
		tlsSecretLister: tlsSecretLister,
		tlsMgrConfig:    tlsMgrConfig,
		cachedcert:      cert,
	}, nil
}

func (t *tlsreloader) GetCertificate(*tls.ClientHelloInfo) (*tls.Certificate, error) {
	t.logger.Info("fetching certificates from getCertificate function")
	select {
	case certs := <-*t.tlsinformer:
		cert, err := fetchTLSCertsFromSecret(t.tlsSecretLister, t.tlsMgrConfig)
		if err != nil {
			return nil, err
		}
		t.cachedcert = cert

		t.logger.V(2).Info(fmt.Sprintf("TLS certs renewed cert=%v key=%v", certs.Cert, certs.Key))
	default:
		t.logger.V(2).Info("TLS certs not renewed")
	}

	if t.cachedcert == nil {
		return nil, errors.Errorf("TLS certs not found")
	}
	return t.cachedcert, nil
}

func fetchTLSCertsFromSecret(tlsSecretLister corelistersv1.SecretLister, tlsMgrConfig *tlsMgr.Config) (*tls.Certificate, error) {
	secret, err := tlsSecretLister.Secrets(tlsMgrConfig.Namespace).Get(tlsMgr.GenerateTLSPairSecretName(tlsMgrConfig))
	if err != nil {
		return nil, err
	}

	if secret == nil {
		return nil, errors.Errorf("secret %s not found", tlsMgr.GenerateTLSPairSecretName(tlsMgrConfig))
	}
	keyBytes := secret.Data[corev1.TLSPrivateKeyKey]
	block, _ := pem.Decode(keyBytes)
	key, err := x509.ParsePKCS1PrivateKey(block.Bytes)
	if err != nil {
		return nil, err
	}

	certBytes := secret.Data[corev1.TLSCertKey]
	cert := pemToCertificates(certBytes)

	err = writeTLSCerts(tlsMgr.TLSCerts{
		Key:  key,
		Cert: cert[0],
	})
	if err != nil {
		return nil, err
	}

	tlscert, err := tls.LoadX509KeyPair(knvTypes.CertFile, knvTypes.KeyFile)
	if err != nil {
		return nil, err
	}

	if err != nil {
		return nil, err
	}
	return &tlscert, nil
}

func pemToCertificates(raw []byte) []*x509.Certificate {
	var certs []*x509.Certificate
	for {
		certPemBlock, next := pem.Decode(raw)
		if certPemBlock == nil {
			return certs
		}
		raw = next
		cert, err := x509.ParseCertificate(certPemBlock.Bytes)
		if err == nil {
			certs = append(certs, cert)
		}
	}
}

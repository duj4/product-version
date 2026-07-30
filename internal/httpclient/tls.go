package httpclient

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net/http"
	"os"
	"time"
)

// NewTLSClient creates an HTTP client configured for mutual TLS.
func NewTLSClient(caCertPath, clientCertPath, clientKeyPath string, timeout time.Duration) (*http.Client, error) {
	if caCertPath == "" || clientCertPath == "" || clientKeyPath == "" {
		return nil, fmt.Errorf("TLS requires CA, client certificate, and client key paths")
	}

	tlsConfig, err := loadTLSConfig(caCertPath, clientCertPath, clientKeyPath)
	if err != nil {
		return nil, err
	}

	return &http.Client{
		Timeout:   normalizeTimeout(timeout),
		Transport: newTransport(tlsConfig),
	}, nil
}

func loadTLSConfig(caCertPath, clientCertPath, clientKeyPath string) (*tls.Config, error) {
	caCert, err := os.ReadFile(caCertPath)
	if err != nil {
		return nil, fmt.Errorf("read CA certificate %q: %w", caCertPath, err)
	}

	caPool := x509.NewCertPool()
	if ok := caPool.AppendCertsFromPEM(caCert); !ok {
		return nil, fmt.Errorf("append CA certificate %q", caCertPath)
	}

	cert, err := tls.LoadX509KeyPair(clientCertPath, clientKeyPath)
	if err != nil {
		return nil, fmt.Errorf("load client certificate %q and key %q: %w", clientCertPath, clientKeyPath, err)
	}

	return &tls.Config{
		RootCAs:      caPool,
		Certificates: []tls.Certificate{cert},
		MinVersion:   tls.VersionTLS12,
	}, nil
}

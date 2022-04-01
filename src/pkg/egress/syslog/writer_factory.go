package syslog

import (
	"encoding/base64"
	"crypto/tls"
	"errors"
	"fmt"

	metrics "code.cloudfoundry.org/go-metric-registry"
	"code.cloudfoundry.org/loggregator-agent-release/src/pkg/egress"
)

type metricClient interface {
	NewCounter(name, helpText string, o ...metrics.MetricOption) metrics.Counter
}

type WriterFactory struct {
	internalTlsConfig *tls.Config
	externalTlsConfig *tls.Config
	egressMetric      metrics.Counter
	netConf           NetworkTimeoutConfig
}

func NewWriterFactory(internalTlsConfig *tls.Config, externalTlsConfig *tls.Config, netConf NetworkTimeoutConfig, m metricClient) WriterFactory {
	metric := m.NewCounter(
		"egress",
		"Total number of envelopes successfully egressed.",
	)
	return WriterFactory{
		internalTlsConfig: internalTlsConfig,
		externalTlsConfig: externalTlsConfig,
		egressMetric:      metric,
		netConf:           netConf,
	}
}

func (f WriterFactory) NewWriter(
	urlBinding *URLBinding,
) (egress.WriteCloser, error) {
	var o []ConverterOption
	if urlBinding.OmitMetadata == true {
		o = append(o, WithoutSyslogMetadata())
	}
	tlsConfig := f.externalTlsConfig
	if urlBinding.InternalTls == true {
		tlsConfig = f.internalTlsConfig
	}
	converter := NewConverter(o...)

	var err error
	var w egress.WriteCloser

	if len(urlBinding.Cert) > 0 && len(urlBinding.Key) > 0 {
            certBytes, _ := base64.StdEncoding.DecodeString(urlBinding.Cert)
            keyBytes, _ := base64.StdEncoding.DecodeString(urlBinding.Key)
            cert, certErr := tls.X509KeyPair(certBytes, keyBytes)
            if certErr != nil {
                err = errors.New(fmt.Sprintf("Failed to load certificate: %s", certErr))
            }
            // clone tls before assign certificate. Otherwise the certificate is used for all connections
            tlsConfig = tlsConfig.Clone()
            tlsConfig.Certificates = []tls.Certificate{cert}
        }

        if err != nil {
		return nil, err
	}

	switch urlBinding.URL.Scheme {
	case "https":
		w, err = NewHTTPSWriter(
			urlBinding,
			f.netConf,
			tlsConfig,
			f.egressMetric,
			converter,
		), nil
	case "syslog":
		w, err = NewTCPWriter(
			urlBinding,
			f.netConf,
			f.egressMetric,
			converter,
		), nil
	case "syslog-tls":
            w, err = NewTLSWriter(
                urlBinding,
                f.netConf,
                tlsConfig,
                f.egressMetric,
                converter,
            ), nil
	}

	if err != nil {
		return nil, err
	}

	if w == nil {
		return nil, errors.New(fmt.Sprintf("unsupported protocol: %v", urlBinding.URL.Scheme))
	}

	return NewRetryWriter(
		urlBinding,
		ExponentialDuration,
		maxRetries,
		w,
	)
}

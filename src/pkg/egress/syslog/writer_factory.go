package syslog

import (
	"crypto/tls"
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
	if urlBinding.OmitMetadata {
		o = append(o, WithoutSyslogMetadata())
	}
	tlsConfig := f.externalTlsConfig
	if urlBinding.InternalTls {
		tlsConfig = f.internalTlsConfig
	}
	converter := NewConverter(o...)

	var err error
	var w egress.WriteCloser
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
		// if len(urlBinding.Certificate) > 0 && len(urlBinding.PrivateKey) > 0 {
		// 	credentials, err := tls.X509KeyPair(urlBinding.Certificate, urlBinding.PrivateKey)
		// 	if err != nil {
		// 		log.Fatalf("Failed to load certificate: %s", err)
		// 	}
		// 	tlsConfig.Certificates = []tls.Certificate{credentials}
		// }
		w, err = NewTLSWriter(
			urlBinding,
			f.netConf,
			tlsConfig,
			f.egressMetric,
			converter,
		), nil
	}

	if w == nil {
		return nil, fmt.Errorf("unsupported protocol: %s", urlBinding.URL.Scheme)
	}

	if err != nil {
		return nil, err
	}

	return NewRetryWriter(
		urlBinding,
		ExponentialDuration,
		maxRetries,
		w,
	)
}

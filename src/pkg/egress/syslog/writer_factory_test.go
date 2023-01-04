package syslog_test

import (
	"crypto/tls"
	"net/url"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	metricsHelpers "code.cloudfoundry.org/go-metric-registry/testhelpers"
	"code.cloudfoundry.org/loggregator-agent-release/src/pkg/egress/syslog"
)

var _ = Describe("EgressFactory", func() {
	var (
		f  syslog.WriterFactory
		sm *metricsHelpers.SpyMetricsRegistry
	)

	BeforeEach(func() {
		sm = metricsHelpers.NewMetricsRegistry()
		f = syslog.NewWriterFactory(&tls.Config{}, &tls.Config{}, syslog.NetworkTimeoutConfig{}, sm) //nolint:gosec
	})

	Context("when the url begins with https", func() {
		It("returns an https writer", func() {
			url, err := url.Parse("https://syslog.example.com")
			Expect(err).ToNot(HaveOccurred())
			urlBinding := &syslog.URLBinding{
				URL: url,
			}

			writer, err := f.NewWriter(urlBinding)
			Expect(err).ToNot(HaveOccurred())

			retryWriter, ok := writer.(*syslog.RetryWriter)
			Expect(ok).To(BeTrue())

			_, ok = retryWriter.Writer.(*syslog.HTTPSWriter)
			Expect(ok).To(BeTrue())

			metric := sm.GetMetric("egress", nil)
			Expect(metric).ToNot(BeNil())
		})
	})

	Context("when the url begins with syslog://", func() {
		It("returns a tcp writer", func() {
			url, err := url.Parse("syslog://syslog.example.com")
			Expect(err).ToNot(HaveOccurred())
			urlBinding := &syslog.URLBinding{
				URL: url,
			}

			writer, err := f.NewWriter(urlBinding)
			Expect(err).ToNot(HaveOccurred())

			retryWriter, ok := writer.(*syslog.RetryWriter)
			Expect(ok).To(BeTrue())

			_, ok = retryWriter.Writer.(*syslog.TCPWriter)
			Expect(ok).To(BeTrue())

			metric := sm.GetMetric("egress", nil)
			Expect(metric).ToNot(BeNil())
		})
	})

	Context("when the url begins with syslog-tls://", func() {
		It("returns a syslog-tls writer", func() {
			url, err := url.Parse("syslog-tls://syslog.example.com")
			Expect(err).ToNot(HaveOccurred())
			urlBinding := &syslog.URLBinding{
				URL: url,
			}

			writer, err := f.NewWriter(urlBinding)
			Expect(err).ToNot(HaveOccurred())

			retryWriter, ok := writer.(*syslog.RetryWriter)
			Expect(ok).To(BeTrue())

			_, ok = retryWriter.Writer.(*syslog.TLSWriter)
			Expect(ok).To(BeTrue())
			metric := sm.GetMetric("egress", nil)
			Expect(metric).ToNot(BeNil())
		})

		Context("when the certificate or private key is invalid", func() {
			It("returns an error", func() {
				url, err := url.Parse("syslog-tls://syslog.example.com")
				Expect(err).ToNot(HaveOccurred())
				urlBinding := &syslog.URLBinding{
					URL:         url,
					Certificate: []byte("invalid-certificate"),
					PrivateKey:  []byte("invalid-private-key"),
				}

				_, err = f.NewWriter(urlBinding)
				Expect(err).To(MatchError(MatchRegexp(
					`failed to load certificate: tls:.*, binding: "syslog-tls://syslog.example.com"`)))
			})
		})

		Context("when the private key is not passed", func() {
			It("the certificate is ignored", func() {
				url, err := url.Parse("syslog-tls://syslog.example.com")
				Expect(err).ToNot(HaveOccurred())
				urlBinding := &syslog.URLBinding{
					URL:         url,
					Certificate: []byte("invalid-certificate"),
				}

				_, err = f.NewWriter(urlBinding)
				Expect(err).ToNot(HaveOccurred())
			})
		})

		Context("when the certificate is not passed", func() {
			It("the private key is ignored", func() {
				url, err := url.Parse("syslog-tls://syslog.example.com")
				Expect(err).ToNot(HaveOccurred())
				urlBinding := &syslog.URLBinding{
					URL:        url,
					PrivateKey: []byte("invalid-private-key"),
				}

				_, err = f.NewWriter(urlBinding)
				Expect(err).ToNot(HaveOccurred())
			})
		})

		Context("when the certificate authority is invalid", func() {
			It("returns an error", func() {
				url, err := url.Parse("syslog-tls://syslog.example.com")
				Expect(err).ToNot(HaveOccurred())
				urlBinding := &syslog.URLBinding{
					URL: url,
					CA:  []byte("invalid-ca-certificate"),
				}

				_, err = f.NewWriter(urlBinding)
				Expect(err).To(MatchError(MatchRegexp(
					`failed to load root ca, binding: "syslog-tls://syslog.example.com"`)))
			})
		})
	})

	Context("when the binding has an invalid scheme", func() {
		It("returns an error", func() {
			url, err := url.Parse("invalid://syslog.example.com")
			Expect(err).ToNot(HaveOccurred())
			urlBinding := &syslog.URLBinding{
				URL: url,
			}

			_, err = f.NewWriter(urlBinding)
			Expect(err).To(MatchError(`unsupported protocol: "invalid", binding: "invalid://syslog.example.com"`))
		})
	})
})

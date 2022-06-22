package binding_test

import (
	metricsHelpers "code.cloudfoundry.org/go-metric-registry/testhelpers"
	"code.cloudfoundry.org/loggregator-agent-release/src/pkg/binding"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("Store", func() {
	It("should store and retrieve bindings", func() {
		store := binding.NewStore(metricsHelpers.NewMetricsRegistry())
		bindings := binding.BindingsMap{
			"app-1": binding.Binding{
				AppID: "app-1",
				Drains: []binding.Drain{
					{
						Url: "syslog://app-1-syslog",
					},
				},
				Hostname: "host-1",
			},
		}

		store.SetNonMtls(bindings)
		store.Merge(binding.MergeBindings)
		Expect(store.Get()).Should(ContainElement(bindings["app-1"]))
	})

	It("should store and retrieve mtls bindings", func() {
		store := binding.NewStore(metricsHelpers.NewMetricsRegistry())
		bindings := binding.BindingsMap{
			"app-1": binding.Binding{
				AppID: "app-1",
				Drains: []binding.Drain{
					{
						Url: "syslog-mtls://app-1-syslog",
						TLSCredential: binding.TLSCredential{
							Cert: "a cert",
							Key:  "a key",
						},
					},
				},
				Hostname: "host-1",
			},
		}

		store.SetMtls(bindings)
		store.Merge(binding.MergeBindings)
		Expect(store.Get()).Should(ContainElement(bindings["app-1"]))
	})

	It("should not return nil bindings", func() {
		store := binding.NewStore(metricsHelpers.NewMetricsRegistry())
		Expect(store.Get()).ToNot(BeNil())
	})

	It("should not allow setting of bindings to nil", func() {
		store := binding.NewStore(metricsHelpers.NewMetricsRegistry())

		nonMtlsBindings := binding.BindingsMap{
			"app-1": binding.Binding{
				AppID: "app-1",
				Drains: []binding.Drain{
					{
						Url: "syslog://app-1-syslog",
					},
				},
				Hostname: "host-1",
			},
		}

		mtlsBindings := binding.BindingsMap{
			"app-1": binding.Binding{
				AppID: "app-1",
				Drains: []binding.Drain{
					{
						Url: "syslog-mtls://app-1-syslog",
						TLSCredential: binding.TLSCredential{
							Cert: "a cert",
							Key:  "a key",
						},
					},
				},
				Hostname: "host-1",
			},
		}
		store.SetNonMtls(nonMtlsBindings)
		store.SetMtls(mtlsBindings)
		store.Merge(binding.MergeBindings)
		store.SetNonMtls(nil)
		store.SetMtls(nil)
		store.Merge(binding.MergeBindings)

		storedBindings := store.Get()
		Expect(storedBindings).ToNot(BeNil())
		Expect(storedBindings).To(BeEmpty())
	})

	// The race detector will cause a failure here
	// if the store is not thread safe
	It("should be thread safe", func() {
		store := binding.NewStore(metricsHelpers.NewMetricsRegistry())

		go func() {
			for i := 0; i < 1000; i++ {
				nonMtlsBindings := binding.BindingsMap{
					"app-1": binding.Binding{
						AppID: "app-1",
						Drains: []binding.Drain{
							{
								Url: "syslog://app-1-syslog",
							},
						},
						Hostname: "host-1",
					},
				}

				mtlsBindings := binding.BindingsMap{
					"app-1": binding.Binding{
						AppID: "app-1",
						Drains: []binding.Drain{
							{
								Url: "syslog-mtls://app-1-syslog",
								TLSCredential: binding.TLSCredential{
									Cert: "a cert",
									Key:  "a key",
								},
							},
						},
						Hostname: "host-1",
					},
				}
				store.SetNonMtls(nonMtlsBindings)
				store.SetMtls(mtlsBindings)
				store.Merge(binding.MergeBindings)
			}
		}()

		for i := 0; i < 1000; i++ {
			_ = store.Get()
		}
	})

	It("tracks the number of bindings", func() {
		metrics := metricsHelpers.NewMetricsRegistry()
		store := binding.NewStore(metrics)
		nonMtlsBindings := binding.BindingsMap{
			"app-1": binding.Binding{
				AppID: "app-1",
				Drains: []binding.Drain{
					{
						Url: "syslog://app-1-syslog",
					},
				},
				Hostname: "host-1",
			},
		}

		mtlsBindings := binding.BindingsMap{
			"app-1": binding.Binding{
				AppID: "app-1",
				Drains: []binding.Drain{
					{
						Url: "syslog-mtls://app-1-syslog",
						TLSCredential: binding.TLSCredential{
							Cert: "a cert",
							Key:  "a key",
						},
					},
				},
				Hostname: "host-1",
			},
		}
		store.SetNonMtls(nonMtlsBindings)
		store.SetMtls(mtlsBindings)
		store.Merge(binding.MergeBindings)

		Expect(metrics.GetMetric("cached_bindings", nil).Value()).
			To(BeNumerically("==", 1))
	})
})

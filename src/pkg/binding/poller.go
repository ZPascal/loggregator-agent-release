package binding

import (
	"encoding/json"
	"log"
	"net/http"
	"time"

	metrics "code.cloudfoundry.org/go-metric-registry"
)

type Poller struct {
	apiClient               client
	pollingInterval         time.Duration
	mtlsPollingInterval     time.Duration
	bindingsProcessInterval time.Duration
	store                   Setter

	logger                     *log.Logger
	bindingRefreshErrorCounter metrics.Counter
	lastBindingCount           metrics.Gauge
	lastMtlsBindingCount       metrics.Gauge
}

type client interface {
	GetUrls(int) (*http.Response, error)
	GetCerts() (*http.Response, error)
}

type Binding struct {
	AppID    string  `json:"app_id"`
	Drains   []Drain `json:"drains"`
	Hostname string  `json:"hostname"`
}

type Drain struct {
	Url           string        `json:"url"`
	TLSCredential TLSCredential `json:"tls_credential"`
}

type TLSCredential struct {
	Cert string `json:"cert"`
	Key  string `json:"key"`
}

type BindingsMap map[string]Binding

type Setter interface {
	Merge(f func(nonMtlsBindings, mtlsBindings BindingsMap) []Binding)
	SetNonMtls(bindings BindingsMap)
	SetMtls(bindings BindingsMap)
}

func NewPoller(ac client, pi, mtlsPi, bpi time.Duration, s Setter, m Metrics, logger *log.Logger) *Poller {
	p := &Poller{
		apiClient:               ac,
		pollingInterval:         pi,
		mtlsPollingInterval:     mtlsPi,
		bindingsProcessInterval: bpi,
		store:                   s,
		logger:                  logger,
		bindingRefreshErrorCounter: m.NewCounter(
			"binding_refresh_error",
			"Total number of failed requests to the binding provider.",
		),
		lastBindingCount: m.NewGauge(
			"last_binding_refresh_count",
			"Current number of bindings received from binding provider during last refresh.",
		),
		lastMtlsBindingCount: m.NewGauge(
			"last_mtls_binding_refresh_count",
			"Current number of mtls bindings received from binding provider during last refresh.",
		),
	}
	p.pollBindings()
	p.pollMtlsBindings()
	p.store.Merge(MergeBindings)
	return p
}

func (p *Poller) Poll() {
	t := time.NewTicker(p.pollingInterval)

	for range t.C {
		err := p.pollBindings()
		if err != nil {
			continue
		}
	}
}

func (p *Poller) MtlsPoll() {
	t := time.NewTicker(p.mtlsPollingInterval)

	for range t.C {
		err := p.pollMtlsBindings()
		if err != nil {
			continue
		}
	}
}

func (p *Poller) Process() {
	t := time.NewTicker(p.bindingsProcessInterval)

	for range t.C {
		p.store.Merge(MergeBindings)
	}
}

func (p *Poller) pollBindings() error {
	nextID := 0
	bindings := make(BindingsMap)
	for {
		resp, err := p.apiClient.GetUrls(nextID)
		if err != nil {
			p.bindingRefreshErrorCounter.Add(1)
			p.logger.Printf("failed to get id %d from CUPS Provider: %s", nextID, err)
			return err
		}
		var aResp apiResponse
		err = json.NewDecoder(resp.Body).Decode(&aResp)
		if err != nil {
			p.logger.Printf("failed to decode JSON: %s", err)
			return err
		}

		bindings = p.toResults(bindings, aResp)

		nextID = aResp.NextID

		if nextID == 0 {
			break
		}
	}

	p.lastBindingCount.Set(float64(len(bindings)))
	p.store.SetNonMtls(bindings)

	return nil
}

func (p *Poller) pollMtlsBindings() error {
	bindings := make(BindingsMap, 0)
	resp, err := p.apiClient.GetCerts()
	if err != nil {
		p.bindingRefreshErrorCounter.Add(1)
		p.logger.Printf("failed to get mtls Bindings from CUPS Provider: %s", err)
		return err
	}
	var aResp certApiResponse
	err = json.NewDecoder(resp.Body).Decode(&aResp)
	if err != nil {
		p.logger.Printf("failed to decode JSON: %s", err)
		return err
	}
	p.lastMtlsBindingCount.Set(float64(len(bindings)))
	p.store.SetMtls(bindings)

	return nil
}

func (p *Poller) toResults(bindings BindingsMap, aResp apiResponse) BindingsMap {
	for k, v := range aResp.Results {
		var drains []Drain
		for _, d := range v.Drains {
			drains = append(drains, Drain{
				Url: d,
			})
		}
		bindings[k] = Binding{
			AppID:    k,
			Drains:   drains,
			Hostname: v.Hostname,
		}
	}
	return bindings
}

func MergeBindings(nonMtlsBindings, mtlsBindings BindingsMap) []Binding {
	bindings := make(BindingsMap, 0)
	bindingsStore := make([]Binding, 0)

	for k, v := range nonMtlsBindings {
		bindings[k] = v
	}
	for k, v := range mtlsBindings {
		if nonMtlsBinding, found := nonMtlsBindings[k]; found {
			drains := append(v.Drains, nonMtlsBinding.Drains...)
			bindings[k] = Binding{
				AppID:    k,
				Drains:   drains,
				Hostname: v.Hostname,
			}
		} else {
			bindings[k] = v
		}
	}
	for _, v := range bindings {
		bindingsStore = append(bindingsStore, v)
	}
	return bindingsStore
}

type apiResponse struct {
	Results Results `json:"results"`
	NextID  int     `json:"next_id"`
}

type Results map[string]struct {
	Drains   []string `json:"drains"`
	Hostname string   `json:"hostname"`
}

type certApiResponse struct {
	Bindings BindingsMap `json:"bindings"`
}

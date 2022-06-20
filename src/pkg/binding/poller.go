package binding

import (
	"encoding/json"
	"log"
	"net/http"
	"time"

	metrics "code.cloudfoundry.org/go-metric-registry"
)

type Poller struct {
	apiClient       client
	pollingInterval time.Duration
	store           Setter

	logger                     *log.Logger
	bindingRefreshErrorCounter metrics.Counter
	lastBindingCount           metrics.Gauge
}

type client interface {
	GetUrls(int) (*http.Response, error)
	GetCerts(time.Time) (*http.Response, error)
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

type Setter interface {
	Set([]Binding)
}

func NewPoller(ac client, pi time.Duration, s Setter, m Metrics, logger *log.Logger) *Poller {
	p := &Poller{
		apiClient:       ac,
		pollingInterval: pi,
		store:           s,
		logger:          logger,
		bindingRefreshErrorCounter: m.NewCounter(
			"binding_refresh_error",
			"Total number of failed requests to the binding provider.",
		),
		lastBindingCount: m.NewGauge(
			"last_binding_refresh_count",
			"Current number of bindings received from binding provider during last refresh.",
		),
	}
	p.poll()
	return p
}

func (p *Poller) Poll() {
	t := time.NewTicker(p.pollingInterval)

	for range t.C {
		p.poll()
	}
}

func (p *Poller) poll() {
	nextID := 0
	results := make(map[string]Binding)

	for {
		resp, err := p.apiClient.GetUrls(nextID)
		if err != nil {
			p.bindingRefreshErrorCounter.Add(1)
			p.logger.Printf("failed to get id %d from CUPS Provider: %s", nextID, err)
			return
		}
		var aResp apiResponse
		err = json.NewDecoder(resp.Body).Decode(&aResp)
		if err != nil {
			p.logger.Printf("failed to decode JSON: %s", err)
			return
		}

		results = p.toResults(results, aResp)

		nextID = aResp.NextID

		if nextID == 0 {
			break
		}
	}

	resp, err := p.apiClient.GetCerts(time.Time{})
	if err != nil {
		p.bindingRefreshErrorCounter.Add(1)
		p.logger.Printf("failed to get id %d from CUPS Provider: %s", nextID, err)
		return
	}
	var aResp certApiResponse
	err = json.NewDecoder(resp.Body).Decode(&aResp)
	if err != nil {
		p.logger.Printf("failed to decode JSON: %s", err)
		return
	}

	results = p.mergeCertificates(results, aResp.Bindings)

	bindings := p.toBindings(results)
	p.lastBindingCount.Set(float64(len(bindings)))
	p.store.Set(bindings)
}

func (p *Poller) toBindings(results map[string]Binding) []Binding {
	bindings := make([]Binding, 0)
	for _, v := range results {
		bindings = append(bindings, v)
	}
	return bindings
}

func (p *Poller) toResults(results map[string]Binding, aResp apiResponse) map[string]Binding {
	for k, v := range aResp.Results {
		var drains []Drain
		for _, d := range v.Drains {
			drains = append(drains, Drain{
				Url: d,
			})
		}
		results[k] = Binding{
			AppID:    k,
			Drains:   drains,
			Hostname: v.Hostname,
		}
	}
	return results
}

func (p *Poller) mergeCertificates(bindings map[string]Binding, mtlsBindings map[string]Binding) map[string]Binding {
	if mtlsBindings == nil {
		return bindings
	}

	for bk, bv := range mtlsBindings {
		if binding, shouldMerge := bindings[bk]; shouldMerge {
			drains := append(binding.Drains, bv.Drains...)
			bindings[bk] = Binding{
				AppID:    binding.AppID,
				Drains:   drains,
				Hostname: binding.Hostname,
			}
		} else {
			bindings[bk] = bv
		}
	}
	return bindings
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
	LastUpdate time.Time          `json:"last_update"`
	Bindings   map[string]Binding `json:"bindings"`
}

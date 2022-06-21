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

type bindingsMap map[string]Binding

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
	bindings, err := p.pollBindings()
	if err != nil {
		return
	}
	bindings, err = p.pollMtlsBindings(bindings)
	if err != nil {
		return
	}
	p.storeBindings(bindings)
}

func (p *Poller) pollBindings() (bindingsMap, error) {
	nextID := 0
	bindings := make(bindingsMap)

	for {
		resp, err := p.apiClient.GetUrls(nextID)
		if err != nil {
			p.bindingRefreshErrorCounter.Add(1)
			p.logger.Printf("failed to get id %d from CUPS Provider: %s", nextID, err)
			return nil, err
		}
		var aResp apiResponse
		err = json.NewDecoder(resp.Body).Decode(&aResp)
		if err != nil {
			p.logger.Printf("failed to decode JSON: %s", err)
			return nil, err
		}

		bindings = p.toResults(bindings, aResp)

		nextID = aResp.NextID

		if nextID == 0 {
			break
		}
	}

	return bindings, nil
}

func (p *Poller) pollMtlsBindings(bindings bindingsMap) (bindingsMap, error) {
	resp, err := p.apiClient.GetCerts(time.Time{})
	if err != nil {
		p.bindingRefreshErrorCounter.Add(1)
		p.logger.Printf("failed to get mtls Bindings from CUPS Provider: %s", err)
		return nil, err
	}
	var aResp certApiResponse
	err = json.NewDecoder(resp.Body).Decode(&aResp)
	if err != nil {
		p.logger.Printf("failed to decode JSON: %s", err)
		return nil, err
	}

	bindings = p.mergeMtlsBindings(bindings, aResp.Bindings)

	return bindings, nil
}

func (p *Poller) storeBindings(fetchedBindings bindingsMap) {
	bindings := make([]Binding, 0)
	for _, v := range fetchedBindings {
		bindings = append(bindings, v)
	}
	p.lastBindingCount.Set(float64(len(bindings)))
	p.store.Set(bindings)

}

func (p *Poller) toResults(bindings bindingsMap, aResp apiResponse) bindingsMap {
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

func (p *Poller) mergeMtlsBindings(bindings bindingsMap, mtlsBindings bindingsMap) bindingsMap {
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
	LastUpdate time.Time   `json:"last_update"`
	Bindings   bindingsMap `json:"bindings"`
}

package bindings

import (
	"net/url"

	"code.cloudfoundry.org/loggregator-agent-release/src/pkg/egress/syslog"
)

type CacheFetcher interface {
	GetAggregate() ([]string, error)
}

type AggregateDrainFetcher struct {
	bindings []string
	cf       CacheFetcher
}

func NewAggregateDrainFetcher(bindings []string, cf CacheFetcher) *AggregateDrainFetcher {
	drainFetcher := &AggregateDrainFetcher{cf: cf}
	drainFetcher.bindings = bindings
	return drainFetcher
}

func (a *AggregateDrainFetcher) FetchBindings() ([]syslog.Binding, error) {
	if len(a.bindings) != 0 {
		var bindings []syslog.Binding
		for _, binding := range a.bindings {
			bindings = append(bindings, syslog.Binding{Drain: syslog.Drain{Url: binding}})
		}
		return bindings, nil
	} else if a.cf != nil {
		aggregate, err := a.cf.GetAggregate()
		if err != nil {
			return []syslog.Binding{}, err
		}
		syslogBindings := []syslog.Binding{}
		syslogBindings = append(syslogBindings, parseBindings(aggregate)...)
		return syslogBindings, nil
	} else {
		return []syslog.Binding{}, nil
	}
}

func parseBindings(urls []string) []syslog.Binding {
	syslogBindings := []syslog.Binding{}
	for _, b := range urls {
		if b == "" {
			continue
		}
		bindingType := syslog.BINDING_TYPE_LOG
		urlParsed, err := url.Parse(b)
		if err != nil {
			continue
		}
		if urlParsed.Query().Get("include-metrics-deprecated") != "" {
			bindingType = syslog.BINDING_TYPE_AGGREGATE
		}
		binding := syslog.Binding{
			AppId: "",
			Drain: syslog.Drain{Url: b},
			Type:  bindingType,
		}
		syslogBindings = append(syslogBindings, binding)
	}
	return syslogBindings
}

func (a *AggregateDrainFetcher) DrainLimit() int {
	return -1
}

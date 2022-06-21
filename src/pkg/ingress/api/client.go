package api

import (
	"fmt"
	"net/http"
)

var (
	sDUrlsPathTemplate  = "%s/internal/v4/syslog_drain_urls?batch_size=%d&next_id=%d"
	sDCertspathTemplate = "%s/internal/v4/mtls_syslog_drain_urls"
)

type Client struct {
	Client    *http.Client
	Addr      string
	BatchSize int
}

func (w Client) GetUrls(nextID int) (*http.Response, error) {
	return w.Client.Get(fmt.Sprintf(sDUrlsPathTemplate, w.Addr, w.BatchSize, nextID))
}

func (w Client) GetCerts() (*http.Response, error) {
	return w.Client.Get(fmt.Sprintf(sDCertspathTemplate, w.Addr))
}

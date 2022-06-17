package api

import (
	"fmt"
	"net/http"
	"time"
)

var (
	sDUrlsPathTemplate  = "%s/internal/v4/syslog_drain_urls?batch_size=%d&next_id=%d"
	sDCertspathTemplate = "%s/internal/v4/syslog_drain_certs?updated_since=%s"
)

type Client struct {
	Client    *http.Client
	Addr      string
	BatchSize int
}

func (w Client) GetUrls(nextID int) (*http.Response, error) {
	return w.Client.Get(fmt.Sprintf(sDUrlsPathTemplate, w.Addr, w.BatchSize, nextID))
}

func (w Client) GetCerts(updatedSince time.Time) (*http.Response, error) {
	return w.Client.Get(fmt.Sprintf(sDCertspathTemplate, w.Addr, updatedSince.Format(time.RFC3339)))
}

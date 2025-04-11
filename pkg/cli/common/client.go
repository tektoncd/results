package common

import (
	"net/url"
	"time"

	"k8s.io/client-go/transport"
)

// Config for the HTTP client
type Config struct {
	URL       *url.URL
	Timeout   time.Duration
	Transport *transport.Config
}

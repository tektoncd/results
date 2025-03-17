package common

import (
	"k8s.io/client-go/transport"
	"net/url"
	"time"
)

type Config struct {
	URL       *url.URL
	Timeout   time.Duration
	Transport *transport.Config
}

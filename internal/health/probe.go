package health

import (
	"net/http"
	"sync/atomic"
)

const (
	probeNotReady int32 = 0
	probeReady    int32 = 1
)

type Probe struct {
	name   string
	status int32
}

func NewProbe(name string) *Probe {
	return &Probe{name: name}
}

func (p *Probe) Name() string { return p.name }
func (p *Probe) Enable()      { atomic.StoreInt32(&p.status, probeReady) }
func (p *Probe) Disable()     { atomic.StoreInt32(&p.status, probeNotReady) }
func (p *Probe) Status() bool { return atomic.LoadInt32(&p.status) == probeReady }

func (p *Probe) ServeHTTP(w http.ResponseWriter, _ *http.Request) {
	if p.Status() {
		w.WriteHeader(http.StatusOK)
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		_, _ = w.Write([]byte("ok"))

		return
	}

	w.WriteHeader(http.StatusServiceUnavailable)
}

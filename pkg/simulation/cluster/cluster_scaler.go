package cluster

import (
	"context"
	"math/rand"
	"time"

	"istio.io/pkg/log"

	"github.com/howardjohn/pilot-load/pkg/simulation/model"
)

type ClusterScaler struct {
	Cluster *Cluster
	cancel  context.CancelFunc
	done    chan struct{}
}

func makeTicker(t time.Duration) <-chan time.Time {
	if t <= 0 {
		// Fake timer
		return make(chan time.Time)
	}
	return time.NewTicker(t).C
}

func (s *ClusterScaler) Run(ctx model.Context) error {
	c, cancel := context.WithCancel(ctx.Context)
	s.cancel = cancel
	s.done = make(chan struct{})
	go func() {
		defer close(s.done)
		instanceJitterT := makeTicker(time.Duration(s.Cluster.Spec.Config.Jitter.Workloads))
		for {
			// TODO: more customization around everything here
			select {
			case <-c.Done():
				return
			case <-instanceJitterT:
				wls := []model.RefreshableSimulation{}
				for _, ns := range s.Cluster.namespaces {
					for _, w := range ns.deployments {
						wls = append(wls, w)
					}
				}
				if err := wls[rand.Intn(len(wls))].Refresh(ctx); err != nil {
					log.Errorf("failed to jitter workload: %v", err)
				}
			}
		}
	}()
	return nil
}

func (s *ClusterScaler) Cleanup(ctx model.Context) error {
	if s == nil {
		return nil
	}
	s.cancel()
	<-s.done
	return nil
}

var _ model.Simulation = &ClusterScaler{}

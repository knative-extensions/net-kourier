/*
 Copyright 2020 The Knative Authors
 Licensed under the Apache License, Version 2.0 (the "License");
 you may not use this file except in compliance with the License.
 You may obtain a copy of the License at

     http://www.apache.org/licenses/LICENSE-2.0

 Unless required by applicable law or agreed to in writing, software
 distributed under the License is distributed on an "AS IS" BASIS,
 WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 See the License for the specific language governing permissions and
 limitations under the License.

*/

package ingress

import (
	"context"
	"crypto/tls"
	"fmt"
	"kourier/pkg/config"
	"net"
	"net/http"
	"reflect"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"knative.dev/serving/pkg/reconciler/ingress/resources"

	"knative.dev/pkg/system"

	"knative.dev/serving/pkg/apis/networking/v1alpha1"

	"go.uber.org/zap"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	corev1listers "k8s.io/client-go/listers/core/v1"
	"k8s.io/client-go/util/workqueue"

	"knative.dev/serving/pkg/network/prober"
)

const (
	probeConcurrency = 5
	stateExpiration  = 5 * time.Minute
	cleanupPeriod    = 1 * time.Minute
	probeTimeout     = 1 * time.Second
)

var dialContext = (&net.Dialer{Timeout: probeTimeout}).DialContext

// ingressState represents the probing progress at the Ingress scope
type ingressState struct {
	id      string
	ingress *v1alpha1.Ingress
	// pendingCount is the number of pods that haven't been successfully probed yet
	pendingCount int32
	lastAccessed time.Time

	cancel func()
}

// podState represents the probing progress at the Pod scope
type podState struct {
	successCount int32

	context context.Context
	cancel  func()
}

type workItem struct {
	ingressState *ingressState
	podState     *podState
	url          string
	podIP        string
	hostname     string
}

// StatusProber provides a way to check if a VirtualService is ready by probing the Envoy pods
// handling that VirtualService.
type StatusProber struct {
	logger *zap.SugaredLogger

	// mu guards snapshotStates and podStates
	mu            sync.Mutex
	ingressStates map[string]*ingressState
	podStates     map[string]*podState

	workQueue       workqueue.RateLimitingInterface
	endpointsLister corev1listers.EndpointsLister

	readyCallback    func(ingress *v1alpha1.Ingress)
	probeConcurrency int
	stateExpiration  time.Duration
	cleanupPeriod    time.Duration
}

// NewStatusProber creates a new instance of StatusProber
func NewStatusProber(
	logger *zap.SugaredLogger,
	endpointsLister corev1listers.EndpointsLister,
	readyCallback func(ingress *v1alpha1.Ingress)) *StatusProber {
	return &StatusProber{
		logger:        logger,
		ingressStates: make(map[string]*ingressState),
		podStates:     make(map[string]*podState),
		workQueue: workqueue.NewNamedRateLimitingQueue(
			workqueue.DefaultControllerRateLimiter(),
			"ProbingQueue"),
		readyCallback:    readyCallback,
		endpointsLister:  endpointsLister,
		probeConcurrency: probeConcurrency,
		stateExpiration:  stateExpiration,
		cleanupPeriod:    cleanupPeriod,
	}
}

// IsReady checks if the provided Ingress is ready, i.e. the Envoy pods serving the Ingress
// have all been updated. This function is designed to be used by the Ingress controller, i.e. it
// will be called in the order of reconciliation. This means that if IsReady is called on an Ingress,
// this Ingress is the latest known version and therefore anything related to older versions can be ignored.
// Also, it means that IsReady is not called concurrently.
func (m *StatusProber) IsReady(ingress *v1alpha1.Ingress) (bool, error) {
	hash, err := resources.ComputeIngressHash(ingress)
	if err != nil {
		return false, err
	}

	ingressKey := fmt.Sprintf("%x", hash)

	if ready, ok := func() (bool, bool) {
		m.mu.Lock()
		defer m.mu.Unlock()
		if state, ok := m.ingressStates[ingressKey]; ok {
			if state.id == ingressKey {
				state.lastAccessed = time.Now()
				return atomic.LoadInt32(&state.pendingCount) == 0, true
			}

			// Cancel the polling for the outdated version
			state.cancel()
			delete(m.ingressStates, ingressKey)
		}
		return false, false
	}(); ok {
		return ready, nil
	}

	ingCtx, cancel := context.WithCancel(context.Background())
	snapshotState := &ingressState{
		id:           ingressKey,
		ingress:      ingress,
		pendingCount: 0,
		lastAccessed: time.Now(),
		cancel:       cancel,
	}

	var workItems []*workItem
	eps, err := m.endpointsLister.Endpoints(system.Namespace()).Get(config.InternalServiceName)
	if err != nil {
		return false, fmt.Errorf("failed to get internal service: %w", err)
	}

	var readyIPs []string
	for _, sub := range eps.Subsets {
		for _, address := range sub.Addresses {
			readyIPs = append(readyIPs, address.IP)
		}
	}

	if len(readyIPs) == 0 {
		return false, fmt.Errorf("no gateway pods available")
	}

	for _, ip := range readyIPs {
		ctx, cancel := context.WithCancel(ingCtx)
		podState := &podState{
			successCount: 0,
			context:      ctx,
			cancel:       cancel,
		}
		// Save the podState to be able to cancel it in case of Pod deletion
		func() {
			m.mu.Lock()
			defer m.mu.Unlock()
			m.podStates[ip] = podState
		}()

		// Update states and cleanup m.podStates when probing is done or cancelled
		go func(ip string) {
			<-podState.context.Done()
			m.updateStates(snapshotState, podState)

			m.mu.Lock()
			defer m.mu.Unlock()
			// It is critical to check that the current podState is also the one stored in the map
			// before deleting it because it could have been replaced if a new version of the ingress
			// has started being probed.
			if state, ok := m.podStates[ip]; ok && state == podState {
				delete(m.podStates, ip)
			}
		}(ip)

		port := strconv.Itoa(int(config.HTTPPortInternal))

		workItem := &workItem{
			ingressState: snapshotState,
			podState:     podState,
			url:          "http://" + ip + ":" + port + config.InternalKourierPath + "/" + ingressKey,
			podIP:        ip,
			hostname:     config.InternalKourierDomain,
		}
		workItems = append(workItems, workItem)

	}

	snapshotState.pendingCount += int32(len(readyIPs))

	func() {
		m.mu.Lock()
		defer m.mu.Unlock()
		m.ingressStates[ingressKey] = snapshotState
	}()
	for _, workItem := range workItems {
		m.workQueue.AddRateLimited(workItem)
		m.logger.Infof("Queuing probe for %s, IP: %s (depth: %d)", workItem.url, workItem.podIP, m.workQueue.Len())
	}
	return len(workItems) == 0, nil
}

// Start starts the StatusManager background operations
func (m *StatusProber) Start(done <-chan struct{}) {
	// Start the worker goroutines
	for i := 0; i < m.probeConcurrency; i++ {
		go func() {
			for m.processWorkItem() {
			}
		}()
	}

	// Cleanup the states periodically
	go wait.Until(m.expireOldStates, m.cleanupPeriod, done)

	// Stop processing the queue when cancelled
	go func() {
		<-done
		m.workQueue.ShutDown()
	}()
}

// CancelIngress cancels probing of the provided Ingress.
func (m *StatusProber) CancelIngress(ingress *v1alpha1.Ingress) {
	m.mu.Lock()
	defer m.mu.Unlock()
	hash, err := resources.ComputeIngressHash(ingress)
	if err != nil {
		m.logger.Errorf("failed to compute ingress Hash: %s", err)
	}
	ingressKey := fmt.Sprintf("%x", hash)
	if state, ok := m.ingressStates[ingressKey]; ok {
		state.cancel()
	}
}

// CancelPodProbing cancels probing of the provided Pod IP.
func (m *StatusProber) CancelPodProbing(pod *corev1.Pod) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if state, ok := m.podStates[pod.Status.PodIP]; ok {
		state.cancel()
	}
}

// expireOldStates removes the states that haven't been accessed in a while.
func (m *StatusProber) expireOldStates() {
	m.mu.Lock()
	defer m.mu.Unlock()
	for key, state := range m.ingressStates {
		if time.Since(state.lastAccessed) > m.stateExpiration {
			state.cancel()
			delete(m.ingressStates, key)
		}
	}
}

// processWorkItem processes a single work item from workQueue.
// It returns false when there is no more items to process, true otherwise.
func (m *StatusProber) processWorkItem() bool {
	obj, shutdown := m.workQueue.Get()
	if shutdown {
		return false
	}

	defer m.workQueue.Done(obj)

	// Crash if the item is not of the expected type
	item, ok := obj.(*workItem)
	if !ok {
		m.logger.Fatalf("Unexpected work item type: want: %s, got: %s\n", reflect.TypeOf(&workItem{}).Name(), reflect.TypeOf(obj).Name())
	}
	m.logger.Infof("Processing probe for %s, IP: %s (depth: %d)", item.url, item.podIP, m.workQueue.Len())

	transport := &http.Transport{
		TLSClientConfig: &tls.Config{
			// We only want to know that the Gateway is configured, not that the configuration is valid.
			// Therefore, we can safely ignore any TLS certificate validation.
			InsecureSkipVerify: true,
		},
		DialContext: func(ctx context.Context, network, addr string) (conn net.Conn, e error) {
			// Requests with the IP as hostname and the Host header set do no pass client-side validation
			// because the HTTP client validates that the hostname (not the Host header) matches the server
			// TLS certificate Common Name or Alternative Names. Therefore, http.Request.URL is set to the
			// hostname and it is substituted it here with the target IP.
			return dialContext(ctx, network, net.JoinHostPort(item.podIP, strconv.Itoa(int(config.HTTPPortInternal))))
		}}

	ok, err := prober.Do(
		item.podState.context,
		transport,
		item.url,
		prober.WithHost(config.InternalKourierDomain),
		prober.ExpectsStatusCodes([]int{http.StatusOK}),
	)

	// In case of cancellation, drop the work item
	select {
	case <-item.podState.context.Done():
		m.workQueue.Forget(obj)
		return true
	default:
	}

	if err != nil || !ok {
		// In case of error, enqueue for retry
		m.workQueue.AddRateLimited(obj)
		m.logger.Errorf("Probing of %s failed, IP: %s, ready: %t, error: %v (depth: %d)", item.url, item.podIP, ok, err, m.workQueue.Len())
	} else {
		m.updateStates(item.ingressState, item.podState)
	}
	return true
}

func (m *StatusProber) updateStates(ingressState *ingressState, podState *podState) {
	if atomic.AddInt32(&podState.successCount, 1) == 1 {
		// This is the first successful probe call for the pod, cancel all other work items for this pod
		podState.cancel()

		// This is the last pod being successfully probed, the Ingress is ready
		if atomic.AddInt32(&ingressState.pendingCount, -1) == 0 {
			m.readyCallback(ingressState.ingress)
		}
	}
}

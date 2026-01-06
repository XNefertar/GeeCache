package geecache

import (
	"context"
	"fmt"
	"geecache/consistenthash"
	pb "geecache/geecachepb"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"sync"
	"time"

	"google.golang.org/protobuf/proto"
)

// K8sPeerPicker implements PeerPicker with DNS-based service discovery for Kubernetes
type K8sPeerPicker struct {
	self          string
	basePath      string
	dnsName       string
	port          string
	mu            sync.RWMutex
	peers         *consistenthash.Map
	httpGetters   map[string]*k8sHTTPGetter
	refreshTicker *time.Ticker
	ctx           context.Context
	cancel        context.CancelFunc
}

// k8sHTTPGetter wraps the HTTP getter with health tracking
type k8sHTTPGetter struct {
	baseURL string
	healthy bool
	mu      sync.RWMutex
}

// NewK8sPeerPicker creates a new Kubernetes-aware peer picker
// dnsName: the Kubernetes headless service DNS name (e.g., "geecache-headless.default.svc.cluster.local")
// self: the address of this peer (e.g., "http://pod-name:port")
// port: the port to use for peer communication
func NewK8sPeerPicker(dnsName, self, port string) *K8sPeerPicker {
	// Input validation
	if dnsName == "" {
		log.Fatal("[K8sPeerPicker] DNS name cannot be empty")
	}
	if self == "" {
		log.Fatal("[K8sPeerPicker] Self address cannot be empty")
	}
	if port == "" {
		log.Fatal("[K8sPeerPicker] Port cannot be empty")
	}

	ctx, cancel := context.WithCancel(context.Background())
	p := &K8sPeerPicker{
		self:        self,
		basePath:    "/_geecache/",
		dnsName:     dnsName,
		port:        port,
		httpGetters: make(map[string]*k8sHTTPGetter),
		ctx:         ctx,
		cancel:      cancel,
	}

	// Initialize with empty consistent hash
	p.peers = consistenthash.New(50, nil)

	// Initial discovery
	if err := p.discoverPeers(); err != nil {
		log.Printf("[K8sPeerPicker] Initial discovery failed: %v", err)
	}

	// Start background refresh
	p.refreshTicker = time.NewTicker(10 * time.Second)
	go p.refreshLoop()

	return p
}

// discoverPeers resolves DNS to find all peers
func (p *K8sPeerPicker) discoverPeers() error {
	// Create context with timeout for DNS lookup
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Resolve DNS to get all pod IPs
	resolver := &net.Resolver{}
	ips, err := resolver.LookupHost(ctx, p.dnsName)
	if err != nil {
		return fmt.Errorf("DNS lookup failed for %s: %w", p.dnsName, err)
	}

	if len(ips) == 0 {
		return fmt.Errorf("no peers found via DNS: %s", p.dnsName)
	}

	// Build peer list with http:// prefix and port
	var peers []string
	newGetters := make(map[string]*k8sHTTPGetter)

	for _, ip := range ips {
		peerAddr := fmt.Sprintf("http://%s:%s", ip, p.port)
		if peerAddr == p.self {
			continue // Skip self
		}
		peers = append(peers, peerAddr)

		// Reuse existing getter or create new one
		p.mu.RLock()
		if getter, exists := p.httpGetters[peerAddr]; exists {
			newGetters[peerAddr] = getter
		} else {
			newGetters[peerAddr] = &k8sHTTPGetter{
				baseURL: peerAddr + p.basePath,
				healthy: true,
			}
		}
		p.mu.RUnlock()
	}

	// Update peers atomically
	p.mu.Lock()
	defer p.mu.Unlock()

	p.peers = consistenthash.New(50, nil)
	p.peers.Add(peers...)
	p.httpGetters = newGetters

	log.Printf("[K8sPeerPicker] Discovered %d peers via DNS %s", len(peers), p.dnsName)
	return nil
}

// refreshLoop periodically refreshes the peer list
func (p *K8sPeerPicker) refreshLoop() {
	for {
		select {
		case <-p.ctx.Done():
			return
		case <-p.refreshTicker.C:
			if err := p.discoverPeers(); err != nil {
				log.Printf("[K8sPeerPicker] Refresh failed: %v", err)
			}
		}
	}
}

// PickPeer selects a peer based on the key
func (p *K8sPeerPicker) PickPeer(key string) (PeerGetter, bool) {
	p.mu.RLock()
	defer p.mu.RUnlock()

	if peer := p.peers.Get(key); peer != "" && peer != p.self {
		if getter, ok := p.httpGetters[peer]; ok && getter.isHealthy() {
			return getter, true
		}
	}
	return nil, false
}

// GetAllPeers returns all known peers
func (p *K8sPeerPicker) GetAllPeers() []PeerGetter {
	p.mu.RLock()
	defer p.mu.RUnlock()

	var peers []PeerGetter
	for _, peer := range p.peers.List() {
		if peer != p.self {
			if getter, ok := p.httpGetters[peer]; ok && getter.isHealthy() {
				peers = append(peers, getter)
			}
		}
	}
	return peers
}

// Stop gracefully stops the peer picker
func (p *K8sPeerPicker) Stop() {
	if p.refreshTicker != nil {
		p.refreshTicker.Stop()
	}
	if p.cancel != nil {
		p.cancel()
	}
}

// k8sHTTPGetter methods

func (g *k8sHTTPGetter) Get(context context.Context, in *pb.Request, out *pb.Response) error {
	u := fmt.Sprintf(
		"%v%v/%v",
		g.baseURL,
		url.QueryEscape(in.Group),
		url.QueryEscape(in.Key),
	)
	res, err := http.Get(u)
	if err != nil {
		g.setHealthy(false)
		return err
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		return fmt.Errorf("server returned: %v", res.Status)
	}

	bytes, err := io.ReadAll(res.Body)
	if err != nil {
		return fmt.Errorf("reading response body: %v", err)
	}

	if err = proto.Unmarshal(bytes, out); err != nil {
		return fmt.Errorf("decoding response body: %v", err)
	}

	g.setHealthy(true)
	return nil
}

func (g *k8sHTTPGetter) Remove(context context.Context, in *pb.Request) error {
	u := fmt.Sprintf(
		"%v%v/%v",
		g.baseURL,
		url.QueryEscape(in.Group),
		url.QueryEscape(in.Key),
	)
	req, err := http.NewRequest(http.MethodDelete, u, nil)
	if err != nil {
		return err
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		g.setHealthy(false)
		return err
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		return fmt.Errorf("server returned: %v", res.Status)
	}
	g.setHealthy(true)
	return nil
}

func (g *k8sHTTPGetter) isHealthy() bool {
	g.mu.RLock()
	defer g.mu.RUnlock()
	return g.healthy
}

func (g *k8sHTTPGetter) setHealthy(healthy bool) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.healthy = healthy
}

var _ PeerPicker = (*K8sPeerPicker)(nil)
var _ PeerGetter = (*k8sHTTPGetter)(nil)

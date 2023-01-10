package apex

import (
	"fmt"
	"math/rand"
	"net"
	"strconv"
	"time"

	"github.com/redhat-et/apex/internal/models"
	"golang.zx2c4.com/wireguard/wgctrl"
	"golang.zx2c4.com/wireguard/wgctrl/wgtypes"
)

// handlePeerTunnel build wg tunnels
func (ax *Apex) handlePeerTunnel(wgPeerConfig wgPeerConfig) {
	// validate the endpoint host:port pair parses.
	// temporary: currently if relay state has not converged the endpoint can be registered as (none)
	_, _, err := net.SplitHostPort(wgPeerConfig.Endpoint)
	if err != nil {
		ax.logger.Debugf("failed parse the endpoint address for node [ %s ] (likely still converging) : %v\n", wgPeerConfig.PublicKey, err)
		return
	}

	if err := ax.addPeer(wgPeerConfig); err != nil {
		ax.logger.Errorf("peer tunnel addition failed: %v\n", err)
	}
}

// addPeer add a wg peer
func (ax *Apex) addPeer(wgPeerConfig wgPeerConfig) error {
	wgClient, err := wgctrl.New()
	if err != nil {
		return err
	}
	defer wgClient.Close()

	pubKey, err := wgtypes.ParseKey(wgPeerConfig.PublicKey)
	if err != nil {
		return err
	}

	allowedIP := make([]net.IPNet, len(wgPeerConfig.AllowedIPs))
	for i := range wgPeerConfig.AllowedIPs {
		_, ipNet, err := net.ParseCIDR(wgPeerConfig.AllowedIPs[i])
		if err != nil {
			return err
		}
		allowedIP[i] = *ipNet
	}

	endpointIP, endpointPort, err := net.SplitHostPort(wgPeerConfig.Endpoint)
	if err != nil {
		ax.logger.Debugf("failed parse the endpoint address for node [ %s ] (likely still converging) : %v\n", wgPeerConfig.PublicKey, err)
		return err
	}

	port, err := strconv.Atoi(endpointPort)
	if err != nil {
		return err
	}

	udpAddr := &net.UDPAddr{
		IP:   net.ParseIP(endpointIP),
		Port: port,
	}

	interval := time.Second * 0

	// relay nodes do not set explicit endpoints
	cfg := wgtypes.Config{}
	if ax.hubRouter {
		cfg = wgtypes.Config{
			ReplacePeers: false,
			Peers: []wgtypes.PeerConfig{
				{
					PublicKey:                   pubKey,
					Remove:                      false,
					AllowedIPs:                  allowedIP,
					PersistentKeepaliveInterval: &interval,
				},
			},
		}
	}
	// all other nodes set peer endpoints
	if !ax.hubRouter {
		cfg = wgtypes.Config{
			ReplacePeers: false,
			Peers: []wgtypes.PeerConfig{
				{
					PublicKey:                   pubKey,
					Remove:                      false,
					Endpoint:                    udpAddr,
					AllowedIPs:                  allowedIP,
					PersistentKeepaliveInterval: &interval,
				},
			},
		}
	}

	return wgClient.ConfigureDevice(ax.tunnelIface, cfg)
}

func (ax *Apex) handlePeerDelete(peerListing []models.Peer) error {
	// if the canonical peer listing does not contain a peer from cache, delete the peer
	for _, p := range ax.peerCache {
		if inPeerListing(peerListing, p) {
			continue
		}
		ax.logger.Debugf("Deleting peer with key: %s\n", ax.keyCache[p.DeviceID])
		if err := ax.deletePeer(ax.keyCache[p.DeviceID], ax.tunnelIface); err != nil {
			return fmt.Errorf("failed to delete peer: %v", err)
		}
		// delete the peer route(s)
		ax.handlePeerRouteDelete(ax.tunnelIface, p)
		// remove peer from local peer and key cache
		delete(ax.peerCache, p.ID)
		delete(ax.keyCache, p.DeviceID)

	}

	return nil
}

func (ax *Apex) deletePeer(publicKey, dev string) error {
	wgClient, err := wgctrl.New()
	if err != nil {
		return err
	}
	defer wgClient.Close()

	key, err := wgtypes.ParseKey(publicKey)
	if err != nil {
		return fmt.Errorf("failed to parse public key %s %v", publicKey, err)
	}

	cfg := []wgtypes.PeerConfig{
		{
			PublicKey: key,
			Remove:    true,
		},
	}

	err = wgClient.ConfigureDevice(dev, wgtypes.Config{
		ReplacePeers: false,
		Peers:        cfg,
	})

	if err != nil {
		return fmt.Errorf("failed to remove peer with key %s %v", key, err)
	}

	ax.logger.Infof("Removed peer with key %s", key)
	return nil
}

func inPeerListing(peers []models.Peer, p models.Peer) bool {
	for _, peer := range peers {
		if peer.ID == p.ID {
			return true
		}
	}
	return false
}

func getWgListenPort() int {
	min := 32768
	max := 61000
	rand.Seed(time.Now().UnixNano())
	return rand.Intn(max-min) + min
}

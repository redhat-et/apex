package main

import (
	"encoding/json"
	"log"
)

const zoneChannelBlue = "zone-blue"

type PeerListing []Peer

// Peer REST struct
type Peer struct {
	PublicKey  string `json:"PublicKey"`
	EndpointIP string `json:"EndpointIP"`
	AllowedIPs string `json:"AllowedIPs"`
}

// handleMsg deal with streaming msgs
func handleMsg(payload string) PeerListing {
	var peerListing PeerListing
	err := json.Unmarshal([]byte(payload), &peerListing)
	if err != nil {
		log.Printf("[ERROR] UnmarshalMessage: %v\n", err)
		return nil
	}
	return peerListing
}

// parseJaywalkSupervisorConfig this is hacky but assumes there is no local config
// or if there is will overwrite it from the publisher peer listing
func (js *jaywalkState) parseJaywalkSupervisorConfig(peerListing PeerListing) {

	var peers []wgPeerConfig
	var localInterface wgLocalConfig

	for _, value := range peerListing {
		if value.PublicKey == js.nodePubKey {
			js.nodePubKeyInConfig = true
		}
	}
	if !js.nodePubKeyInConfig {
		log.Printf("Public Key for this node %s was not found in the broker update", js.nodePubKey)
	}

	for _, value := range peerListing {
		if value.PublicKey != js.nodePubKey {
			peer := wgPeerConfig{
				value.PublicKey,
				value.EndpointIP,
				[]string{value.AllowedIPs},
			}
			peers = append(peers, peer)
			log.Printf("[DEBUG] Peer Node Configuration - Peer AllowedIPs [ %s ] Peer Endpoint IP [ %s ] Peer Public Key [ %s ]\n",
				value.AllowedIPs,
				value.EndpointIP,
				value.PublicKey)
		}
		if value.PublicKey == js.nodePubKey {
			localInterface = wgLocalConfig{
				cliFlags.wireguardPvtKey,
				value.AllowedIPs,
				cliFlags.listenPort,
				false,
			}
			log.Printf("[DEBUG] Local Node Configuration - Wireguard Local Endpoint IP [ %s ] Port [ %v ] Local Private Key [ %s ]\n",
				localInterface.Address,
				wgListenPort,
				localInterface.PrivateKey)
			// set the node unique local interface configuration
			js.wgConf.Interface = localInterface
		}
	}
	// all peers get appended to state for a published channel
	js.wgConf.Peer = peers
}

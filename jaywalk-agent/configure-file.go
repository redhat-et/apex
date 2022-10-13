package main

import (
	"os"
	"path/filepath"

	"github.com/redhat-et/jaywalking/common"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/viper"
	"gopkg.in/ini.v1"
)

// parseJaywalkConfig extracts the jaywalk toml config and
// builds the wireguard configuration data structs
func (js *jaywalkState) parseJaywalkConfig() {
	// parse toml config
	viper.SetConfigType("toml")
	viper.SetConfigFile(js.jaywalkConfigFile)
	if err := viper.ReadInConfig(); err != nil {
		log.Fatal("Unable to read config file", err)
	}
	var conf ConfigToml
	err := viper.Unmarshal(&conf)
	if err != nil {
		log.Fatal(err)
	}

	var peers []wgPeerConfig
	var localInterface wgLocalConfig

	for _, value := range conf.Peers {
		if value.PublicKey == js.nodePubKey {
			js.nodePubKeyInConfig = true
		}
	}
	if !js.nodePubKeyInConfig {
		log.Printf("Public Key for this node was not found in %s", jaywalkConfig)
	}
	for nodeName, value := range conf.Peers {
		// Parse the [Peers] section
		if value.PublicKey != js.nodePubKey {
			peer := wgPeerConfig{
				value.PublicKey,
				value.EndpointIP,
				value.WireguardIP,
			}
			peers = append(peers, peer)
			log.Printf("Peer Node Configuration [%v] Peer AllowedIPs [%s] Peer Endpoint IP [%s] Peer Public Key [%s]\n",
				nodeName,
				value.WireguardIP,
				value.EndpointIP,
				value.PublicKey)
		}
		// Parse the [Interface] section of the wg config
		if value.PublicKey == js.nodePubKey {
			localInterface = wgLocalConfig{
				value.PrivateKey,
				value.WireguardIP,
				wgListenPort,
				false,
			}
			log.Infof("Local Node Configuration Name [%v] Wireguard Address [%v] Local Endpoint IP [%v] Local Private Key [%v]\n",
				nodeName,
				value.WireguardIP,
				value.EndpointIP,
				value.PrivateKey)
		}
	}
	js.wgConf.Interface = localInterface
	js.wgConf.Peer = peers
}

func (js *jaywalkState) deployWireguardConfig() {
	latestCfg := &wgConfig{
		Interface: js.wgConf.Interface,
		Peer:      js.wgConf.Peer,
	}

	cfg := ini.Empty(ini.LoadOptions{
		AllowNonUniqueSections: true,
	})

	err := ini.ReflectFrom(cfg, latestCfg)
	if err != nil {
		log.Fatal("load ini configuration from struct error")
	}

	switch js.nodeOS {
	case common.Linux.String():
		// wg does not create the OSX config directory by default
		if err = common.CreateDirectory(wgLinuxConfPath); err != nil {
			log.Fatalf("Unable to create the wireguard config directory [%s]: %v", wgDarwinConfPath, err)
		}

		latestConfig := filepath.Join(wgLinuxConfPath, wgConfLatestRev)
		if err = cfg.SaveTo(latestConfig); err != nil {
			log.Fatal("Save latest configuration error", err)
		}
		if js.nodePubKeyInConfig {

			// If no config exists, copy the latest config rev to /etc/wireguard/wg0.tomlConf
			activeConfig := filepath.Join(wgLinuxConfPath, wgConfActive)
			if _, err = os.Stat(activeConfig); err != nil {
				if err = applyWireguardConf(); err != nil {
					log.Fatal(err)
				}
			} else {
				if err = updateWireguardConfig(); err != nil {
					log.Fatal(err)
				}
			}
		}
	case common.Linux.String():
		activeDarwinConfig := filepath.Join(wgDarwinConfPath, wgConfActive)
		if err = cfg.SaveTo(activeDarwinConfig); err != nil {
			log.Fatal("Save latest configuration error", err)
		}

		if js.nodePubKeyInConfig {
			// this will throw an error that can be ignored if an existing interface doesn't exist
			wgOut, err := common.RunCommand("wg-quick", "down", wgIface)
			if err != nil {
				log.Debugf("failed to start the wireguard interface: %v", err)
			}
			log.Debugf("%v\n", wgOut)
			wgOut, err = common.RunCommand("wg-quick", "up", activeDarwinConfig)
			if err != nil {
				log.Printf("failed to start the wireguard interface: %v", err)
			}
			log.Debugf("%v\n", wgOut)
		} else {
			log.Printf("Tunnels not built since the node's public key was found in the configuration")
		}
	}
}

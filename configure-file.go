package main

import (
	"fmt"
	"github.com/spf13/viper"
	"gopkg.in/ini.v1"
	"log"
	"os"
	"path/filepath"
)

// parseJaywalkConfig extracts the jaywalk toml config and
// builds the wireguard configuration data structs
func (js *jaywalkState) parseJaywalkConfig() {
	// parse toml config TODO: move out of main
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
		if value.PublicKey != js.nodePubKey {
			peer := wgPeerConfig{
				value.PublicKey,
				value.EndpointIP,
				[]string{value.WireguardIP},
			}
			peers = append(peers, peer)
			log.Printf("[DEBUG] Peer Node Configuration [%v] Peer AllowedIPs [%s] Peer Endpoint IP [%s] Peer Public Key [%s]\n",
				nodeName,
				value.WireguardIP,
				value.EndpointIP,
				value.PublicKey)
		}
		if value.PublicKey == js.nodePubKey {
			localInterface = wgLocalConfig{
				value.PrivateKey,
				value.WireguardIP,
				wgListenPort,
				false,
			}
			log.Printf("[DEBUG] Local Node Configuration [%v] Wireguard Address [%v] Local Endpoint IP [%v] Local Private Key [%v]\n",
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
	case linux.String():
		// wg does not create the OSX config directory by default
		if err = createDirectory(wgDarwinConfPath); err != nil {
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
	case darwin.String():
		activeDarwinConfig := filepath.Join(wgDarwinConfPath, wgConfActive)
		if err = cfg.SaveTo(activeDarwinConfig); err != nil {
			log.Fatal("Save latest configuration error", err)
		}

		if js.nodePubKeyInConfig {
			wgOut, err := runCommand("wg-quick", "down", wgIface)
			if err != nil {
				log.Printf("failed to start the wireguard interface: %v", err)
			}
			log.Printf("%v\n", wgOut)
			wgOut, err = runCommand("wg-quick", "up", activeDarwinConfig)
			if err != nil {
				log.Printf("failed to start the wireguard interface: %v", err)
			}
			log.Printf("%v\n", wgOut)
		} else {
			log.Printf("Tunnels not built since the node's public key was found in the configuration")
		}
	}
}

// updateWireguardConfig strip and diff the latest rev to the active config
// TODO: use syncconf and manually track routes instead of wg-quick managing them
func updateWireguardConfig() error {
	activeConfig := filepath.Join(wgLinuxConfPath, wgConfActive)
	latestRevConfig := filepath.Join(wgLinuxConfPath, wgConfLatestRev)
	// If no config exists, copy the latest rev config to /etc/wireguard/wg0-latest-rev.conf
	if _, err := os.Stat(activeConfig); err != nil {
		latestConfig := filepath.Join(wgLinuxConfPath, wgConfLatestRev)
		if err := copyFile(latestConfig, activeConfig); err == nil {
			return nil
		} else {
			return err
		}
	}
	// If a config exists, strip and diff the active and latest revision
	stripActiveCfg, err := runCommand("wg-quick", "strip", activeConfig)
	if err != nil {
		return fmt.Errorf("failed to strip the active configuration: %v", err)
	}
	stripLatestRevCfg, err := runCommand("wg-quick", "strip", latestRevConfig)
	if err != nil {
		return fmt.Errorf("failed to strip the latest configuration: %v", err)
	}

	// unmarshall the active and latest configurations
	activePeers, err := unmarshallWireguadCfg(stripActiveCfg)
	if err != nil {
		return err
	}
	revisedPeers, err := unmarshallWireguadCfg(stripLatestRevCfg)
	if err != nil {
		return err
	}
	// diff the configurations and rebuild the tunnel if there has been a change
	if !diffWireguardConfigs(activePeers.Peer, revisedPeers.Peer) {
		log.Printf("Configuration change detected\n")
		if err := applyWireguardConf(); err != nil {
			return err
		}
	}
	// if there is no wg0 interface, apply the configuration
	if !linkExists(wgIface) {
		if err := applyWireguardConf(); err != nil {
			return err
		}
	}
	return nil
}

func applyWireguardConf() error {
	// TODO: deleting the interface is a hammer and creates disruption
	if linkExists(wgIface) {
		if err := delLink(wgIface); err != nil {
			return fmt.Errorf("unable to delete the existing wireguard interface: %v", err)
		}
	}
	activeConfig := filepath.Join(wgLinuxConfPath, wgConfActive)
	latestRevConfig := filepath.Join(wgLinuxConfPath, wgConfLatestRev)
	// copy the latest config rev to /etc/wireguard/wg0.conf
	if err := copyFile(latestRevConfig, activeConfig); err != nil {
		return err
	}
	wgOut, err := runCommand("wg-quick", "up", activeConfig)
	if err != nil {
		return fmt.Errorf("failed to start the wireguard interface: %v", err)
	}
	log.Printf("%v\n", wgOut)
	return nil
}

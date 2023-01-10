//go:build windows

package apex

import (
	"fmt"
	"net"

	"go.uber.org/zap"
)

// RouteExists currently only used for windows build purposes
func RouteExists(s string) (bool, error) {
	return false, nil
}

// AddRoute adds a windows route to the specified interface
func AddRoute(prefix, dev string) error {
	// TODO: replace with powershell
	_, err := RunCommand("netsh", "int", "ipv4", "add", "route", prefix, dev)
	if err != nil {
		return fmt.Errorf("no windows route added: %v", err)
	}

	return nil
}

// discoverLinuxAddress only used for windows build purposes
func discoverLinuxAddress(logger *zap.SugaredLogger, family int) (net.IP, error) {
	return nil, nil
}

func linkExists(wgIface string) bool {
	ifaces, err := net.Interfaces()
	if err != nil {
		fmt.Print(fmt.Errorf("localAddresses: %+v\n", err.Error()))
		return false
	}
	for _, iface := range ifaces {
		if iface.Name == wgIface {
			return true
		}
	}
	return false
}

// delLink only used for windows build purposes
func delLink(wgIface string) (err error) {
	return nil
}

// DeleteRoute deletes a windows route
func DeleteRoute(prefix, dev string) error {
	_, err := RunCommand("netsh", "int", "ipv4", "del", "route", prefix, dev)
	if err != nil {
		return fmt.Errorf("no route deleted: %v", err)
	}

	return nil
}

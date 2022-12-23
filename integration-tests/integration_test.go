//go:build integration
// +build integration

package integration_tests

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/cenkalti/backoff/v4"
	"github.com/redhat-et/apex/internal/models"
	"github.com/stretchr/testify/suite"
	"github.com/testcontainers/testcontainers-go"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest"
)

var providerType testcontainers.ProviderType
var defaultNetwork string
var hostDNSName string
var ipamDriver string

const apexctl = "../dist/apexctl"

func init() {
	if os.Getenv("APEX_TEST_PODMAN") != "" {
		fmt.Println("Using podman")
		providerType = testcontainers.ProviderPodman
		defaultNetwork = "podman"
		ipamDriver = "host-local"
		hostDNSName = "10.88.0.1"
	} else {
		fmt.Println("Using docker")
		providerType = testcontainers.ProviderDocker
		defaultNetwork = "bridge"
		ipamDriver = "default"
		hostDNSName = "172.17.0.1"
	}
}

type ApexIntegrationSuite struct {
	suite.Suite
	logger *zap.SugaredLogger
}

func TestApexIntegrationSuite(t *testing.T) {
	suite.Run(t, new(ApexIntegrationSuite))
}

func (suite *ApexIntegrationSuite) SetupSuite() {
	logger := zaptest.NewLogger(suite.T())
	suite.logger = logger.Sugar()
}

func (suite *ApexIntegrationSuite) TestBasicConnectivity() {
	assert := suite.Assert()
	require := suite.Require()
	parentCtx := context.Background()
	ctx, cancel := context.WithTimeout(parentCtx, 60*time.Second)
	defer cancel()

	token, err := getToken(ctx, "admin@apex.local", "floofykittens")
	require.NoError(err)

	// create the nodes
	node1 := suite.CreateNode(ctx, "node1", []string{defaultNetwork})
	suite.T().Cleanup(func() {
		if err := node1.Terminate(parentCtx); err != nil {
			suite.logger.Errorf("failed to terminate container %v", err)
		}
	})
	node2 := suite.CreateNode(ctx, "node2", []string{defaultNetwork})
	suite.T().Cleanup(func() {
		if err := node2.Terminate(parentCtx); err != nil {
			suite.logger.Errorf("failed to terminate container %v", err)
		}
	})

	// start apex on the nodes
	go suite.runApex(ctx, node1, fmt.Sprintf("--with-token=%s", token))
	go suite.runApex(ctx, node2, fmt.Sprintf("--with-token=%s", token))

	node1IP, err := getContainerIfaceIP(ctx, "wg0", node1)
	require.NoError(err)
	node2IP, err := getContainerIfaceIP(ctx, "wg0", node2)
	require.NoError(err)

	gather := suite.gatherFail(ctx, node1, node2)
	suite.logger.Infof("Pinging %s from node1", node2IP)
	err = ping(ctx, node1, node2IP)
	assert.NoErrorf(err, gather)

	suite.logger.Infof("Pinging %s from node2", node1IP)
	err = ping(ctx, node2, node1IP)
	assert.NoErrorf(err, gather)

	suite.logger.Info("killing apex and re-joining nodes with new keys")
	//kill the apex process on both nodes
	_, err = suite.containerExec(ctx, node1, []string{"killall", "apex"})
	require.NoError(err)
	_, err = suite.containerExec(ctx, node2, []string{"killall", "apex"})
	require.NoError(err)

	// delete only the public key on node1
	_, err = suite.containerExec(ctx, node1, []string{"rm", "/etc/wireguard/public.key"})
	require.NoError(err)
	// delete the entire wireguard directory on node2
	_, err = suite.containerExec(ctx, node2, []string{"rm", "-rf", "/etc/wireguard/"})
	require.NoError(err)

	// start apex on the nodes
	go suite.runApex(ctx, node1, fmt.Sprintf("--with-token=%s", token))
	go suite.runApex(ctx, node2, fmt.Sprintf("--with-token=%s", token))

	var newNode1IP string
	err = backoff.Retry(
		func() error {
			var err error
			newNode1IP, err = getContainerIfaceIP(ctx, "wg0", node1)
			if err != nil {
				return err
			}
			if newNode1IP == node1IP {
				return fmt.Errorf("new node1IP is the same as old ip")
			}
			return nil
		},
		backoff.WithContext(backoff.NewConstantBackOff(1*time.Second), ctx),
	)
	require.NoError(err)

	var newNode2IP string
	err = backoff.Retry(
		func() error {
			var err error
			newNode2IP, err = getContainerIfaceIP(ctx, "wg0", node2)
			if err != nil {
				return err
			}
			if newNode2IP == node2IP {
				return fmt.Errorf("new node1IP is the same as old ip")
			}
			return nil
		},
		backoff.WithContext(backoff.NewConstantBackOff(1*time.Second), ctx),
	)
	require.NoError(err)

	gather = suite.gatherFail(ctx, node1, node2)
	suite.logger.Infof("Pinging %s from node1", newNode2IP)
	err = ping(ctx, node1, newNode2IP)
	assert.NoError(err, gather)

	suite.logger.Infof("Pinging %s from node2", newNode1IP)
	err = ping(ctx, node2, newNode1IP)
	assert.NoErrorf(err, gather)
}

// TestRequestIPDefaultZone tests requesting a specific address in the default zone
func (suite *ApexIntegrationSuite) TestRequestIPDefaultZone() {
	assert := suite.Assert()
	require := suite.Require()

	node1IP := "10.200.0.101"
	node2IP := "10.200.0.102"
	parentCtx := context.Background()
	ctx, cancel := context.WithTimeout(parentCtx, 60*time.Second)
	defer cancel()
	token, err := getToken(ctx, "admin@apex.local", "floofykittens")
	require.NoError(err)

	// create the nodes
	node1 := suite.CreateNode(ctx, "node1", []string{defaultNetwork})
	suite.T().Cleanup(func() {
		if err := node1.Terminate(parentCtx); err != nil {
			suite.logger.Errorf("failed to terminate container %v", err)
		}
	})
	node2 := suite.CreateNode(ctx, "node2", []string{defaultNetwork})
	suite.T().Cleanup(func() {
		if err := node2.Terminate(parentCtx); err != nil {
			suite.logger.Errorf("failed to terminate container %v", err)
		}
	})

	// start apex on the nodes
	go suite.runApex(ctx, node1,
		fmt.Sprintf("--with-token=%s", token),
		fmt.Sprintf("--request-ip=%s", node1IP),
	)
	go suite.runApex(ctx, node2,
		fmt.Sprintf("--with-token=%s", token),
		fmt.Sprintf("--request-ip=%s", node2IP),
	)

	gather := suite.gatherFail(ctx, node1, node2)
	// ping the requested IP address (--request-ip)
	suite.logger.Infof("Pinging %s from node1", node2IP)
	err = ping(ctx, node1, node2IP)
	assert.NoErrorf(err, gather)

	suite.logger.Infof("Pinging %s from node2", node1IP)
	err = ping(ctx, node2, node1IP)
	assert.NoErrorf(err, gather)
}

// TestRequestIPZone tests requesting a specific address in a newly created zone
func (suite *ApexIntegrationSuite) TestRequestIPZone() {
	assert := suite.Assert()
	require := suite.Require()
	parentCtx := context.Background()
	ctx, cancel := context.WithTimeout(parentCtx, 60*time.Second)
	defer cancel()
	token, err := getToken(ctx, "kitteh1@apex.local", "floofykittens")
	require.NoError(err)

	c, err := newClient(ctx, token)
	require.NoError(err)
	// create a new zone
	zoneID, err := c.CreateZone("zone-blue", "zone full of blue things", "10.140.0.0/24", false)
	require.NoError(err)

	// patch the new user into the zone
	_, err = c.MoveCurrentUserToZone(zoneID.ID)
	require.NoError(err)

	node1IP := "10.140.0.101"
	node2IP := "10.140.0.102"

	// create the nodes
	node1 := suite.CreateNode(ctx, "node1", []string{defaultNetwork})
	suite.T().Cleanup(func() {
		if err := node1.Terminate(parentCtx); err != nil {
			suite.logger.Errorf("failed to terminate container %v", err)
		}
	})
	node2 := suite.CreateNode(ctx, "node2", []string{defaultNetwork})
	suite.T().Cleanup(func() {
		if err := node2.Terminate(parentCtx); err != nil {
			suite.logger.Errorf("failed to terminate container %v", err)
		}
	})

	// start apex on the nodes
	go suite.runApex(ctx, node1,
		fmt.Sprintf("--with-token=%s", token),
		fmt.Sprintf("--request-ip=%s", node1IP),
	)

	go suite.runApex(ctx, node2,
		fmt.Sprintf("--with-token=%s", token),
		fmt.Sprintf("--request-ip=%s", node2IP),
	)

	gather := suite.gatherFail(ctx, node1, node2)

	// ping the requested IP address (--request-ip)
	suite.logger.Infof("Pinging %s from node1", node2IP)
	err = ping(ctx, node1, node2IP)
	assert.NoErrorf(err, gather)

	suite.logger.Infof("Pinging %s from node2", node1IP)
	err = ping(ctx, node2, node1IP)
	assert.NoErrorf(err, gather)

	suite.logger.Info("killing apex and re-joining nodes")
	//kill the apex process on both nodes
	_, err = suite.containerExec(ctx, node1, []string{"killall", "apex"})
	require.NoError(err)
	_, err = suite.containerExec(ctx, node2, []string{"killall", "apex"})
	require.NoError(err)

	// restart apex and ensure the nodes receive the same re-quested address
	suite.logger.Info("Restarting Apex on two spoke nodes and re-joining")
	go suite.runApex(ctx, node1,
		fmt.Sprintf("--with-token=%s", token),
		fmt.Sprintf("--request-ip=%s", node1IP),
	)

	go suite.runApex(ctx, node2,
		fmt.Sprintf("--with-token=%s", token),
		fmt.Sprintf("--request-ip=%s", node2IP),
	)

	gather = suite.gatherFail(ctx, node1, node2)

	// ping the requested IP address (--request-ip)
	suite.logger.Infof("Pinging %s from node1", node2IP)
	err = ping(ctx, node1, node2IP)
	assert.NoErrorf(err, gather)

	suite.logger.Infof("Pinging %s from node2", node1IP)
	err = ping(ctx, node2, node1IP)
	assert.NoErrorf(err, gather)
}

// TestHubZone test a hub zone with 3 nodes, the first being a relay node
func (suite *ApexIntegrationSuite) TestHubZone() {
	assert := suite.Assert()
	require := suite.Require()
	parentCtx := context.Background()
	ctx, cancel := context.WithTimeout(parentCtx, 90*time.Second)
	defer cancel()
	token, err := getToken(ctx, "kitteh2@apex.local", "floofykittens")
	require.NoError(err)

	c, err := newClient(ctx, token)
	require.NoError(err)

	// create a new zone
	zoneID, err := c.CreateZone("zone-relay", "zone with a relay hub", "10.162.0.0/24", true)
	require.NoError(err)

	// patch the new user into the zone
	_, err = c.MoveCurrentUserToZone(zoneID.ID)
	require.NoError(err)

	// create the nodes
	node1 := suite.CreateNode(ctx, "node1", []string{defaultNetwork})
	suite.T().Cleanup(func() {
		if err := node1.Terminate(parentCtx); err != nil {
			suite.logger.Errorf("failed to terminate container %v", err)
		}
	})
	node2 := suite.CreateNode(ctx, "node2", []string{defaultNetwork})
	suite.T().Cleanup(func() {
		if err := node2.Terminate(parentCtx); err != nil {
			suite.logger.Errorf("failed to terminate container %v", err)
		}
	})
	node3 := suite.CreateNode(ctx, "node3", []string{defaultNetwork})
	suite.T().Cleanup(func() {
		if err := node3.Terminate(parentCtx); err != nil {
			suite.logger.Errorf("failed to terminate container %v", err)
		}
	})

	// start apex on the nodes
	go suite.runApex(ctx, node1,
		"--hub-router",
		fmt.Sprintf("--with-token=%s", token),
	)

	// Ensure the relay node has time to register before joining spokes since it is required for hub-zones
	time.Sleep(time.Second * 10)
	go suite.runApex(ctx, node2, fmt.Sprintf("--with-token=%s", token))
	go suite.runApex(ctx, node3, fmt.Sprintf("--with-token=%s", token))

	node1IP, err := getContainerIfaceIP(ctx, "wg0", node1)
	require.NoError(err)
	node2IP, err := getContainerIfaceIP(ctx, "wg0", node2)
	require.NoError(err)
	node3IP, err := getContainerIfaceIP(ctx, "wg0", node3)
	require.NoError(err)

	gather := suite.gatherFail(ctx, node1, node2, node3)

	suite.logger.Infof("Pinging %s from node1", node2IP)
	err = ping(ctx, node1, node2IP)
	assert.NoErrorf(err, gather)

	suite.logger.Infof("Pinging %s from node1", node3IP)
	err = ping(ctx, node1, node3IP)
	assert.NoErrorf(err, gather)

	suite.logger.Infof("Pinging %s from node3", node1IP)
	err = ping(ctx, node3, node2IP)
	assert.NoErrorf(err, gather)

	suite.logger.Infof("Pinging %s from node2", node3IP)
	err = ping(ctx, node2, node3IP)
	assert.NoErrorf(err, gather)

	hubZoneChildPrefix := "10.188.100.0/24"
	node2ChildPrefixLoopbackNet := "10.188.100.1/32"

	suite.T().Logf("killing apex on node2")

	_, err = suite.containerExec(ctx, node2, []string{"killall", "apex"})
	assert.NoError(err)
	suite.T().Logf("rejoining on node2 with --child-prefix=%s", hubZoneChildPrefix)

	// add a loopback that are contained in the node's child prefix
	_, err = suite.containerExec(ctx, node2, []string{"ip", "addr", "add", node2ChildPrefixLoopbackNet, "dev", "lo"})
	require.NoError(err)

	// re-join and ensure the peer table updates with the new values
	go func() {
		_, err = suite.containerExec(ctx, node2, []string{
			"/bin/apex",
			fmt.Sprintf("--child-prefix=%s", hubZoneChildPrefix),
			fmt.Sprintf("--with-token=%s", token),
			"http://apex.local",
		})
	}()

	// address will be the same, this is just a readiness check for gather data
	node1IP, err = getContainerIfaceIP(ctx, "wg0", node1)
	require.NoError(err)
	node2IP, err = getContainerIfaceIP(ctx, "wg0", node2)
	require.NoError(err)
	node3IP, err = getContainerIfaceIP(ctx, "wg0", node3)
	require.NoError(err)

	gather = suite.gatherFail(ctx, node1, node2, node3)

	// parse the loopback ip from the loopback prefix
	node2LoopbackIP, _, _ := net.ParseCIDR(node2ChildPrefixLoopbackNet)

	suite.T().Logf("Pinging loopback on node2 %s from node3 wg0", node2LoopbackIP.String())
	err = ping(ctx, node3, node2LoopbackIP.String())
	assert.NoErrorf(err, gather)

	suite.logger.Infof("Pinging %s from node1", node3IP)
	err = ping(ctx, node1, node3IP)
	assert.NoErrorf(err, gather)

	suite.logger.Infof("Pinging %s from node3", node1IP)
	err = ping(ctx, node3, node2IP)
	assert.NoErrorf(err, gather)

	suite.logger.Infof("Pinging %s from node2", node3IP)
	err = ping(ctx, node2, node3IP)
	assert.NoErrorf(err, gather)

	// get the peer id for node3
	allPeers, err := suite.runCommand(apexctl,
		"--username", "kitteh2@apex.local",
		"--password", "floofykittens",
		"--output", "json-raw",
		"peer", "list-all",
	)
	var peers []models.Peer
	json.Unmarshal([]byte(allPeers), &peers)
	assert.NoErrorf(err, "apexctl peer list-all error: %v\n", err)

	var peer3ID string
	for _, p := range peers {
		if p.NodeAddress == node1IP {
			node3IP = p.NodeAddress
			peer3ID = p.ID.String()
		}
	}

	// delete the peer node2
	_, err = suite.runCommand(apexctl,
		"--username", "kitteh2@apex.local",
		"--password", "floofykittens",
		"peer", "delete",
		"--peer-id", peer3ID,
	)
	require.NoError(err)

	// this is probably more time than needed for convergence as polling is currently 5s
	time.Sleep(time.Second * 10)
	gather = suite.gatherFail(ctx, node1, node2, node3)

	// verify the deleted peer details are no longer in a peer's tables
	node2routes := suite.routesDump(ctx, node2)
	node2dump := suite.wgDump(ctx, node2)
	if strings.Contains(node2routes, node3IP) {
		assert.Errorf(err, "found deleted peer node still in routing tables of a peer", gather)
	}
	if strings.Contains(node2dump, node3IP) {
		assert.Errorf(err, "found deleted peer node still in wg show wg0 dump tables of a peer", gather)
	}
}

// TestChildPrefix tests requesting a specific address in a newly created zone
func (suite *ApexIntegrationSuite) TestChildPrefix() {
	assert := suite.Assert()
	require := suite.Require()
	parentCtx := context.Background()
	ctx, cancel := context.WithTimeout(parentCtx, 60*time.Second)
	defer cancel()
	token, err := getToken(ctx, "kitteh3@apex.local", "floofykittens")
	require.NoError(err)

	c, err := newClient(ctx, token)
	require.NoError(err)

	// create a new zone
	zoneID, err := c.CreateZone("zone-child-prefix", "zone full of toddler prefixes", "100.64.100.0/24", false)
	require.NoError(err)

	// patch the new user into the zone
	_, err = c.MoveCurrentUserToZone(zoneID.ID)
	require.NoError(err)

	node1LoopbackNet := "172.16.10.101/32"
	node2LoopbackNet := "172.16.20.102/32"
	node1ChildPrefix := "172.16.10.0/24"
	node2ChildPrefix := "172.16.20.0/24"

	// create the nodes
	node1 := suite.CreateNode(ctx, "node1", []string{defaultNetwork})
	suite.T().Cleanup(func() {
		if err := node1.Terminate(parentCtx); err != nil {
			suite.logger.Errorf("failed to terminate container %v", err)
		}
	})
	node2 := suite.CreateNode(ctx, "node2", []string{defaultNetwork})
	suite.T().Cleanup(func() {
		if err := node2.Terminate(parentCtx); err != nil {
			suite.logger.Errorf("failed to terminate container %v", err)
		}
	})

	// start apex on the nodes
	go suite.runApex(ctx, node1,
		fmt.Sprintf("--child-prefix=%s", node1ChildPrefix),
		fmt.Sprintf("--with-token=%s", token),
	)

	go suite.runApex(ctx, node2,
		fmt.Sprintf("--child-prefix=%s", node2ChildPrefix),
		fmt.Sprintf("--with-token=%s", token),
	)

	// add loopbacks to the containers that are contained in the node's child prefix
	_, err = suite.containerExec(ctx, node1, []string{"ip", "addr", "add", node1LoopbackNet, "dev", "lo"})
	require.NoError(err)
	_, err = suite.containerExec(ctx, node2, []string{"ip", "addr", "add", node2LoopbackNet, "dev", "lo"})
	require.NoError(err)

	// parse the loopback ip from the loopback prefix
	node1LoopbackIP, _, _ := net.ParseCIDR(node1LoopbackNet)
	node2LoopbackIP, _, _ := net.ParseCIDR(node2LoopbackNet)

	// address will be the same, this is just a readiness check for gather data
	node1IP, err := getContainerIfaceIP(ctx, "wg0", node1)
	require.NoError(err)
	node2IP, err := getContainerIfaceIP(ctx, "wg0", node2)
	require.NoError(err)

	gather := suite.gatherFail(ctx, node1, node2)

	suite.logger.Infof("Pinging %s from node1", node2IP)
	err = ping(ctx, node1, node2IP)
	assert.NoErrorf(err, gather)

	suite.logger.Infof("Pinging %s from node2", node1IP)
	err = ping(ctx, node2, node1IP)
	assert.NoErrorf(err, gather)

	suite.logger.Infof("Pinging %s from node1", node2LoopbackIP)
	err = ping(ctx, node1, node2LoopbackIP.String())
	assert.NoErrorf(err, gather)

	suite.logger.Infof("Pinging %s from node2", node1LoopbackIP)
	err = ping(ctx, node2, node1LoopbackIP.String())
	assert.NoErrorf(err, gather)
}

/*
The following test sets up a NAT scenario that emulates
two networks that are behind  NAT devices and validates
connectivity between local nodes and the relay node.
Spoke nodes within the same network should peer directly
to one another. This validates nodes that cannot UDP hole
punch and can only peer directly to one another.

	           +----------+
	           |  Relay   |
	           |  Node    |
	           +-+------+-+
	  +----------+      +----------+
	  |                            |
	  |                            |
    +-----------+                 +-----------+
    |   NAT     |                 |   NAT     |
    |   Router  |                 |   Router  |
    ++---------++                 ++---------++
     |         |                   |        |
     |         |                   |        |
+-----+---+   ++--------+   +------+--+   +-+-------+
|  Net1   |   |  Net1   |   |  Net2   |   |  Net2   |
|  Spoke1 |   |  Spoke2 |   |  Spoke1 |   |  Spoke2 |
+---------+   +---------+   +---------+   +---------+
*/
// TestRelayNAT tests end to end and spoke to spoke in an easy NAT environment
func (suite *ApexIntegrationSuite) TestRelayNAT() {
	suite.T().Skip("This test is broken on podman since netavark does some magic masquerading to prevent it working. It's also not too healthy on docker either")
	parentCtx := context.Background()
	assert := suite.Assert()
	require := suite.Require()

	net1 := "net1"
	net2 := "net2"
	relayNodeName := "relay"
	net1Spoke1Name := "net1-spoke1"
	net2Spoke1Name := "net2-spoke1"
	net1Spoke2Name := "net1-spoke2"
	net2Spoke2Name := "net2-spoke2"

	ctx, cancel := context.WithTimeout(parentCtx, 30*time.Second)
	defer cancel()

	// launch a relay node in the default namespace that all spokes can reach
	relayNode := suite.CreateNode(ctx, relayNodeName, []string{defaultNetwork})
	suite.T().Cleanup(func() {
		if err := relayNode.Terminate(parentCtx); err != nil {
			suite.logger.Errorf("failed to terminate container %v", err)
		}
	})

	dNet1 := suite.CreateNetwork(ctx, net1, "100.64.11.0/24")
	suite.T().Cleanup(func() {
		if err := dNet1.Remove(parentCtx); err != nil {
			suite.logger.Infof("failed to remove network: %v", err)
		}
	})

	dNet2 := suite.CreateNetwork(ctx, net2, "100.64.12.0/24")
	suite.T().Cleanup(func() {
		if err := dNet2.Remove(parentCtx); err != nil {
			suite.logger.Infof("failed to remove network: %v", err)
		}
	})

	// launch nat nodes
	natNodeNet1 := suite.CreateNode(ctx, "net1-nat", []string{net1, defaultNetwork})
	suite.T().Cleanup(func() {
		if err := natNodeNet1.Terminate(parentCtx); err != nil {
			suite.logger.Errorf("failed to terminate container %v", err)
		}
	})
	natNodeNet2 := suite.CreateNode(ctx, "net2-nat", []string{net2, defaultNetwork})
	suite.T().Cleanup(func() {
		if err := natNodeNet2.Terminate(parentCtx); err != nil {
			suite.logger.Errorf("failed to terminate container %v", err)
		}
	})

	ctx, cancel = context.WithTimeout(parentCtx, 120*time.Second)
	defer cancel()

	// register the nat node interfaces which will be the gateways for spoke nodes
	gatewayNet1, err := getContainerIfaceIP(ctx, "eth0", natNodeNet1)
	require.NoError(err)

	gatewayNet2, err := getContainerIfaceIP(ctx, "eth0", natNodeNet2)
	require.NoError(err)

	// enable masquerading on the nat nodes
	_, err = suite.containerExec(ctx, natNodeNet1, []string{"iptables", "-A", "FORWARD", "-i", "eth0", "-o", "eth1", "-j", "ACCEPT"})
	require.NoError(err)
	_, err = suite.containerExec(ctx, natNodeNet1, []string{"iptables", "-A", "FORWARD", "-i", "eth1", "-o", "eth0", "-j", "ACCEPT"})
	require.NoError(err)
	_, err = suite.containerExec(ctx, natNodeNet1, []string{"iptables", "-t", "nat", "-A", "POSTROUTING", "-o", "eth1", "-j", "MASQUERADE"})
	require.NoError(err)
	_, err = suite.containerExec(ctx, natNodeNet2, []string{"iptables", "-A", "FORWARD", "-i", "eth0", "-o", "eth1", "-j", "ACCEPT"})
	require.NoError(err)
	_, err = suite.containerExec(ctx, natNodeNet2, []string{"iptables", "-A", "FORWARD", "-i", "eth1", "-o", "eth0", "-j", "ACCEPT"})
	require.NoError(err)
	_, err = suite.containerExec(ctx, natNodeNet2, []string{"iptables", "-t", "nat", "-A", "POSTROUTING", "-o", "eth1", "-j", "MASQUERADE"})
	require.NoError(err)

	// create spoke nodes
	net1SpokeNode1 := suite.CreateNode(ctx, net1Spoke1Name, []string{net1})
	suite.T().Cleanup(func() {
		if err := net1SpokeNode1.Terminate(parentCtx); err != nil {
			suite.logger.Errorf("failed to terminate container %v", err)
		}
	})
	net2SpokeNode1 := suite.CreateNode(ctx, net2Spoke1Name, []string{net2})
	suite.T().Cleanup(func() {
		if err := net2SpokeNode1.Terminate(parentCtx); err != nil {
			suite.logger.Errorf("failed to terminate container %v", err)
		}
	})
	net1SpokeNode2 := suite.CreateNode(ctx, net1Spoke2Name, []string{net1})
	suite.T().Cleanup(func() {
		if err := net1SpokeNode2.Terminate(parentCtx); err != nil {
			suite.logger.Errorf("failed to terminate container %v", err)
		}
	})
	net2SpokeNode2 := suite.CreateNode(ctx, net2Spoke2Name, []string{net2})
	suite.T().Cleanup(func() {
		if err := net2SpokeNode2.Terminate(parentCtx); err != nil {
			suite.logger.Errorf("failed to terminate container %v", err)
		}
	})

	// delete the default route pointing to the nat gateway
	_, err = suite.containerExec(ctx, net1SpokeNode1, []string{"ip", "-4", "route", "flush", "default"})
	require.NoError(err)
	_, err = suite.containerExec(ctx, net2SpokeNode1, []string{"ip", "-4", "route", "flush", "default"})
	require.NoError(err)
	_, err = suite.containerExec(ctx, net1SpokeNode1, []string{"ip", "-4", "route", "add", "default", "via", gatewayNet1})
	require.NoError(err)
	_, err = suite.containerExec(ctx, net2SpokeNode1, []string{"ip", "-4", "route", "add", "default", "via", gatewayNet2})
	require.NoError(err)
	_, err = suite.containerExec(ctx, net1SpokeNode2, []string{"ip", "-4", "route", "flush", "default"})
	require.NoError(err)
	_, err = suite.containerExec(ctx, net2SpokeNode2, []string{"ip", "-4", "route", "flush", "default"})
	require.NoError(err)
	_, err = suite.containerExec(ctx, net1SpokeNode2, []string{"ip", "-4", "route", "add", "default", "via", gatewayNet1})
	require.NoError(err)
	_, err = suite.containerExec(ctx, net2SpokeNode2, []string{"ip", "-4", "route", "add", "default", "via", gatewayNet2})
	require.NoError(err)

	suite.logger.Infof("Validate NAT Infra: Pinging %s from net1-spoke1", hostDNSName)
	err = ping(ctx, net1SpokeNode1, hostDNSName)
	assert.NoError(err)
	suite.logger.Infof("Validate NAT Infra: Pinging %s from net2-spoke1", hostDNSName)
	err = ping(ctx, net2SpokeNode1, hostDNSName)
	assert.NoError(err)
	suite.logger.Infof("Validate NAT Infra: Pinging %s from net1-spoke2", hostDNSName)
	err = ping(ctx, net1SpokeNode2, hostDNSName)
	assert.NoError(err)
	suite.logger.Infof("Validate NAT Infra: Pinging %s from net2-spoke2", hostDNSName)
	err = ping(ctx, net2SpokeNode2, hostDNSName)
	assert.NoError(err)

	token, err := getToken(ctx, "kitteh4@apex.local", "floofykittens")
	require.NoError(err)

	c, err := newClient(ctx, token)
	require.NoError(err)

	// create a new zone
	zoneID, err := c.CreateZone("zone-nat-relay", "nat test zone", "10.29.0.0/24", true)
	require.NoError(err)

	// patch the new user into the zone
	_, err = c.MoveCurrentUserToZone(zoneID.ID)
	require.NoError(err)

	// start apex on the nodes
	go suite.runApex(ctx, relayNode,
		"--hub-router",
		fmt.Sprintf("--with-token=%s", token),
	)

	// ensure the relay node has time to register before joining spokes since it is required for hub-zones
	time.Sleep(time.Second * 10)
	go suite.runApex(ctx, net1SpokeNode1,
		"--relay-only",
		fmt.Sprintf("--with-token=%s", token),
	)
	go suite.runApex(ctx, net2SpokeNode1,
		"--relay-only",
		fmt.Sprintf("--with-token=%s", token),
	)
	go suite.runApex(ctx, net1SpokeNode2,
		"--relay-only",
		fmt.Sprintf("--with-token=%s", token),
	)
	go suite.runApex(ctx, net2SpokeNode2,
		"--relay-only",
		fmt.Sprintf("--with-token=%s", token),
	)
	time.Sleep(time.Second * 10)

	relayNodeIP, err := getContainerIfaceIP(ctx, "wg0", relayNode)
	require.NoError(err)
	net1SpokeNode1IP, err := getContainerIfaceIP(ctx, "wg0", net1SpokeNode1)
	require.NoError(err)
	net2SpokeNode1IP, err := getContainerIfaceIP(ctx, "wg0", net2SpokeNode1)
	require.NoError(err)
	net1SpokeNode2IP, err := getContainerIfaceIP(ctx, "wg0", net1SpokeNode2)
	require.NoError(err)
	net2SpokeNode2IP, err := getContainerIfaceIP(ctx, "wg0", net2SpokeNode2)
	require.NoError(err)

	suite.logger.Infof("Pinging %s %s from %s", net1Spoke1Name, net1SpokeNode1IP, relayNodeName)
	err = ping(ctx, relayNode, net1SpokeNode1IP)
	assert.NoError(err)

	suite.logger.Infof("Pinging %s %s from %s", net2Spoke1Name, net2SpokeNode1IP, relayNodeName)
	err = ping(ctx, relayNode, net2SpokeNode1IP)
	assert.NoError(err)

	suite.logger.Infof("Pinging %s %s from node %s", relayNodeName, relayNodeIP, net1Spoke1Name)
	err = ping(ctx, net1SpokeNode1, relayNodeIP)
	assert.NoError(err)

	suite.logger.Infof("Pinging %s %s from node %s", relayNodeName, relayNodeIP, net2Spoke1Name)
	err = ping(ctx, net2SpokeNode1, relayNodeIP)
	assert.NoError(err)

	suite.logger.Infof("Pinging %s %s from node %s", relayNodeName, relayNodeIP, net1Spoke2Name)
	err = ping(ctx, net1SpokeNode2, relayNodeIP)
	assert.NoError(err)

	suite.logger.Infof("Pinging %s %s from node %s", relayNodeName, relayNodeIP, net2Spoke2Name)
	err = ping(ctx, net2SpokeNode2, relayNodeIP)
	assert.NoError(err)

	suite.logger.Infof("Pinging %s %s from node %s", net1Spoke1Name, net1SpokeNode1IP, net1Spoke2Name)
	err = ping(ctx, net1SpokeNode2, net1SpokeNode1IP)
	assert.NoError(err)

	suite.logger.Infof("Pinging %s %s from node %s", net2Spoke1Name, net2SpokeNode1IP, net2Spoke2Name)
	err = ping(ctx, net2SpokeNode2, net2SpokeNode1IP)
	assert.NoError(err)

	// dump the wg state from the relay node.
	wgShow, err := suite.containerExec(ctx, relayNode, []string{"wg", "show", "wg0", "dump"})
	require.NoError(err)
	suite.logger.Infof("Relay node wireguard state: \n%s", wgShow)

	suite.logger.Info("killing apex and re-joining nodes")

	// kill the apex process on both nodes
	_, err = suite.containerExec(ctx, net1SpokeNode1, []string{"killall", "apex"})
	require.NoError(err)
	_, err = suite.containerExec(ctx, net2SpokeNode1, []string{"killall", "apex"})
	require.NoError(err)

	// restart the process on two nodes and verify re-joining
	suite.logger.Info("Restarting apex on two spoke nodes and re-joining")
	go suite.runApex(ctx, net1SpokeNode1,
		"--relay-only",
		fmt.Sprintf("--with-token=%s", token),
	)
	go suite.runApex(ctx, net2SpokeNode1,
		"--relay-only",
		fmt.Sprintf("--with-token=%s", token),
	)

	suite.logger.Infof("Pinging %s %s from node %s", net1Spoke2Name, net1SpokeNode2IP, net1Spoke1Name)
	err = ping(ctx, net1SpokeNode1, net1SpokeNode2IP)
	assert.NoError(err)

	// validate the re-joined nodes can communicate
	suite.logger.Infof("Pinging %s %s from node %s", net2Spoke2Name, net2SpokeNode2IP, net2Spoke1Name)
	err = ping(ctx, net2SpokeNode1, net2SpokeNode2IP)
	assert.NoError(err)

	// verify there are (n) lines in the wg show output on a spoke node in each network
	wgSpokeShow, err := suite.containerExec(ctx, net1SpokeNode1, []string{"wg", "show", "wg0", "dump"})
	require.NoError(err)
	lc, err := lineCount(wgSpokeShow)
	require.NoError(err)
	assert.Equal(5, lc, "the number of expected wg show peers was %d, found %d: wg show out: \n%s", 5, lc, wgSpokeShow)

	// verify there are (n) lines in the wg show output on a spoke node in each network
	wgSpokeShow, err = suite.containerExec(ctx, net2SpokeNode1, []string{"wg", "show", "wg0", "dump"})
	require.NoError(err)
	lc, err = lineCount(wgSpokeShow)
	require.NoError(err)
	assert.Equal(5, lc, "the number of expected wg show peers was %d, found %d: wg show out: \n%s", 5, lc, wgSpokeShow)
}

func (suite *ApexIntegrationSuite) TestApexCtl() {
	assert := suite.Assert()
	require := suite.Require()
	parentCtx := context.Background()
	ctx, cancel := context.WithTimeout(parentCtx, 60*time.Second)
	defer cancel()
	user := "kitteh5@apex.local"
	pass := "floofykittens"

	token, err := getToken(ctx, user, pass)
	require.NoError(err)

	// create the nodes
	node1 := suite.CreateNode(ctx, "node1", []string{defaultNetwork})
	suite.T().Cleanup(func() {
		if err := node1.Terminate(parentCtx); err != nil {
			suite.logger.Errorf("failed to terminate container %v", err)
		}
	})
	node2 := suite.CreateNode(ctx, "node2", []string{defaultNetwork})
	suite.T().Cleanup(func() {
		if err := node2.Terminate(parentCtx); err != nil {
			suite.logger.Errorf("failed to terminate container %v", err)
		}
	})

	// validate apexctl user list returns a user
	userOut, err := suite.runCommand(apexctl,
		"--username", user,
		"--password", pass,
		"--output", "json",
		"user", "list",
	)
	require.NoErrorf(err, "apexctl user list error: %v\n", err)
	var users []models.User
	err = json.Unmarshal([]byte(userOut), &users)
	assert.NotEmpty(users)

	// create a new zone and parse the returned json for the zone id
	zoneOut, err := suite.runCommand(apexctl,
		"--username", user,
		"--password", pass,
		"--output", "json",
		"zone", "create",
		"--name", "zone-apexctl",
		"--cidr", "172.19.100.0/24",
		"--description", "apexctl e2e zone",
	)
	require.NoErrorf(err, "apexctl zone create error: %v\n", err)
	var zone models.Zone
	err = json.Unmarshal([]byte(zoneOut), &zone)
	assert.NotEmpty(zone.ID.String())

	// move the current user into the new zone
	_, err = suite.runCommand(apexctl,
		"--username", user,
		"--password", pass,
		"zone", "move-user",
		"--zone-id", zone.ID.String(),
	)
	assert.NoErrorf(err, "apexctl zone move-user error: %v\n", err)

	// start apex on the nodes
	go suite.runApex(ctx, node1, fmt.Sprintf("--with-token=%s", token))
	go suite.runApex(ctx, node2, fmt.Sprintf("--with-token=%s", token), "--child-prefix=100.22.100.0/24")

	node1IP, err := getContainerIfaceIP(ctx, "wg0", node1)
	require.NoError(err)
	node2IP, err := getContainerIfaceIP(ctx, "wg0", node2)
	require.NoError(err)

	gather := suite.gatherFail(ctx, node1, node2)
	suite.logger.Infof("Pinging %s from node1", node2IP)
	err = ping(ctx, node1, node2IP)
	assert.NoErrorf(err, gather)

	suite.logger.Infof("Pinging %s from node2", node1IP)
	err = ping(ctx, node2, node1IP)
	assert.NoErrorf(err, gather)

	// validate list-all peers and register IDs and IPs
	allPeers, err := suite.runCommand(apexctl,
		"--username", user,
		"--password", pass,
		"--output", "json-raw",
		"peer", "list-all",
	)
	var peers []models.Peer
	json.Unmarshal([]byte(allPeers), &peers)
	assert.NoErrorf(err, "apexctl peer list-all error: %v\n", err)

	// register the device IDs for node1 and node2
	var node1DeviceID string
	var node2DeviceID string
	for _, p := range peers {
		if p.NodeAddress == node1IP {
			node1DeviceID = p.DeviceID.String()
		}
		if p.NodeAddress == node2IP {
			node2DeviceID = p.DeviceID.String()
		}
	}

	//kill the apex process on both nodes
	_, err = suite.containerExec(ctx, node1, []string{"killall", "apex"})
	require.NoError(err)
	_, err = suite.containerExec(ctx, node2, []string{"killall", "apex"})
	require.NoError(err)

	// delete both devices from apex
	_, err = suite.runCommand(apexctl,
		"--username", user,
		"--password", pass,
		"device", "delete",
		"--device-id", node1DeviceID,
	)
	require.NoError(err)
	_, err = suite.runCommand(apexctl,
		"--username", user,
		"--password", pass,
		"device", "delete",
		"--device-id", node2DeviceID,
	)
	require.NoError(err)

	// delete the keys on both nodes to force ensure the deleted device released it's
	// IPAM address and will re-issue that address to a new device with a new keypair.
	_, err = suite.containerExec(ctx, node1, []string{"rm", "-rf", "/etc/wireguard/"})
	require.NoError(err)
	_, err = suite.containerExec(ctx, node2, []string{"rm", "-rf", "/etc/wireguard/"})
	require.NoError(err)

	time.Sleep(time.Second * 10)
	// re-join both nodes, flipping the child-prefix to node1 to ensure the child-prefix was released
	go suite.runApex(ctx, node1, fmt.Sprintf("--with-token=%s", token), "--child-prefix=100.22.100.0/24")
	go suite.runApex(ctx, node2, fmt.Sprintf("--with-token=%s", token))

	newNode1IP, err := getContainerIfaceIP(ctx, "wg0", node1)
	require.NoError(err)
	gather = suite.gatherFail(ctx, node1, node2)

	// If the device was not deleted, the next registered device would receive the
	// next available address in the IPAM pool, not the previously assigned address.
	var addressMatch bool
	if newNode1IP == node2IP {
		addressMatch = true
		suite.logger.Infof("Pinging %s from node1", node1IP)
		err = ping(ctx, node1, node1IP)
		assert.NoErrorf(err, gather)
	}
	if newNode1IP == node1IP {
		addressMatch = true
		suite.logger.Infof("Pinging %s from node1", node2IP)
		err = ping(ctx, node1, node2IP)
		assert.NoErrorf(err, gather)
	}
	if !addressMatch {
		assert.Failf("ipam/device delete failed", fmt.Sprintf("Node did not receive the proper IPAM address %s, it should have been %s or %s\n %s", newNode1IP, node1IP, node2IP, gather))
	}

	// validate list peers in a zone
	peersInZone, err := suite.runCommand(apexctl,
		"--username", user,
		"--password", pass,
		"--output", "json-raw",
		"peer", "list",
		"--zone-id", zone.ID.String(),
	)

	json.Unmarshal([]byte(peersInZone), &peers)
	assert.NoErrorf(err, "apexctl peer list-all error: %v\n", err)

	// re-register the device IDs for node1 and node2 as they have been re-created w/new IDs
	for _, p := range peers {
		if p.NodeAddress == node1IP {
			node1DeviceID = p.DeviceID.String()
		}
		if p.NodeAddress == node2IP {
			node2DeviceID = p.DeviceID.String()
		}
	}
	// delete all devices from the zone as currently required to avoid sql key
	// constraints, then delete the zone, then recreate the zone to ensure the
	// IPAM prefix was released. If it was not released the creation will fail.
	_, err = suite.runCommand(apexctl,
		"--username", user,
		"--password", pass,
		"device", "delete",
		"--device-id", node1DeviceID,
	)
	require.NoError(err)

	_, err = suite.runCommand(apexctl,
		"--username", user,
		"--password", pass,
		"device", "delete",
		"--device-id", node2DeviceID,
	)
	require.NoError(err)

	// delete the zone
	zoneOut, err = suite.runCommand(apexctl,
		"--username", user,
		"--password", pass,
		"--output", "json",
		"zone", "delete",
		"--zone-id", zone.ID.String(),
	)
	require.NoError(err)

	// re-create the deleted zone, this will fail if the IPAM
	// prefix was not released from the prior deletion
	_, err = suite.runCommand(apexctl,
		"--username", user,
		"--password", pass,
		"zone", "create",
		"--name", "zone-apexctl",
		"--cidr", "172.19.100.0/24",
		"--description", "apexctl e2e zone",
	)
	require.NoError(err)
}

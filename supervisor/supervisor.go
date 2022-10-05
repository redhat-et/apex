package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/go-redis/redis"
	"github.com/google/uuid"
)

var (
	redisDB       *redis.Client
	peers         []Peer
	streamService *string
	streamPasswd  *string
)

const (
	zoneBlue   = "zone-blue"
	streamPort = 6379
)

func init() {
	streamService = flag.String("streamer-address", "", "streamer address")
	streamPasswd = flag.String("streamer-passwd", "", "streamer password")
	flag.Parse()

	streamSocket := fmt.Sprintf("%s:%d", *streamService, streamPort)
	redisDB = redis.NewClient(&redis.Options{
		Addr:     streamSocket,
		Password: *streamPasswd,
	})

}

// Peer represents data about a record Peers.
type Peer struct {
	PublicKey  string `json:"PublicKey"`
	EndpointIP string `json:"EndpointIP"`
	AllowedIPs string `json:"AllowedIPs"`
}

func main() {

	// Connect to the redis channel
	defer redisDB.Close()
	pubsub := redisDB.Subscribe(zoneBlue)
	defer pubsub.Close()

	// Start the gin router
	router := gin.Default()
	router.GET("/peers", getPeers)
	router.GET("/peers/:id", getPeerByKey)
	router.POST("/peers", postPeers)
	router.Run("localhost:8080")
}

// getPeers responds with the list of all peers as JSON.
func getPeers(c *gin.Context) {
	c.IndentedJSON(http.StatusOK, peers)
}

// postPeers adds a Peers from JSON received in the request body.
func postPeers(c *gin.Context) {
	var newPeer []Peer

	// Call BindJSON to bind the received JSON to
	if err := c.BindJSON(&newPeer); err != nil {
		return
	}

	// TODO: add single node config if we use REST
	// Add the new Peers to the slice.
	//peers = append(peers, newPeer)
	c.IndentedJSON(http.StatusCreated, newPeer)

	log.Println("[INFO] New node added, pushing changes to the mesh..")
	//PublishMessage(zoneBlue, newPeer)

	publishAllPeersMessage(zoneBlue, newPeer)

}

// getPeerByKey locates the Peers whose PublicKey value matches the id
// parameter sent by the client, then returns that Peers as a response.
func getPeerByKey(c *gin.Context) {
	id := c.Param("id")

	// Loop through the list of peers, looking for
	// a Peers whose PublicKey value matches the parameter.
	for _, a := range peers {
		if a.PublicKey == id {
			c.IndentedJSON(http.StatusOK, a)
			return
		}
	}
	c.IndentedJSON(http.StatusNotFound, gin.H{"message": "Peers not found"})
}

func publishAllPeersMessage(channel string, data []Peer) {
	id, msg := createAllPeerMessage(data)
	err := redisDB.Publish(channel, msg).Err()
	if err != nil {
		log.Printf("[ERROR] sending %s message failed, %v\n", id, err)
		return
	}
	log.Printf("[INFO] Published new message: %s\n", msg)
}

func createAllPeerMessage(postData []Peer) (string, string) {
	id := uuid.NewString()
	msg, _ := json.Marshal(postData)
	return id, string(msg)
}

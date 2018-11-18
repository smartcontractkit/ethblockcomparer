package main

import (
	"crypto/tls"
	"encoding/json"
	"fmt"
	"log"
	"math/big"
	"net/http"
	"os"
	"strconv"
	"strings"

	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/rpc"
	"github.com/gin-gonic/gin"
	"go.uber.org/multierr"
)

func main() {
	if len(os.Args) != 4 {
		log.Panic("Must pass two Ethereum JSON-RPC Endpoints as arguments to compare block heights, and a threshold that defines a healthy height difference (e.g. 20).")
	}
	http.DefaultTransport.(*http.Transport).TLSClientConfig = &tls.Config{InsecureSkipVerify: insecureSkipVerify()}
	r, err := createRouter(os.Args[1], os.Args[2], os.Args[3])
	checkError(err)
	log.Print("Comparing block heights from ", os.Args[1], " and ", os.Args[2], ", erroring when difference is greater than ", os.Args[3])
	checkError(r.Run())
}

func insecureSkipVerify() bool {
	flag, ok := os.LookupEnv("INSECURE_SKIP_VERIFY")
	if ok {
		b, err := strconv.ParseBool(flag)
		if err != nil {
			log.Print("Error:", err.Error())
		}
		return b
	}
	return false
}

func createRouter(p1, p2, threshold string) (*gin.Engine, error) {
	r := gin.Default()
	hc, err := newHeightsController(p1, p2, threshold)
	if err != nil {
		return nil, err
	}
	r.GET("/heights", hc.Index)
	return r, err
}

func checkError(err error) {
	if err != nil {
		log.Panic(err)
	}
}

type heightsController struct {
	threshold *big.Int
	client1   client
	client2   client
}

func newHeightsController(endpoint1, endpoint2, thresholdStr string) (*heightsController, error) {
	threshold, errd := strconv.ParseUint(thresholdStr, 10, 32)
	c1, err1 := rpc.Dial(normalizeLocalhost(endpoint1))
	c2, err2 := rpc.Dial(normalizeLocalhost(endpoint2))
	merr := multierr.Combine(errd, err1, err2)
	if merr != nil {
		return nil, merr
	}
	return &heightsController{
		threshold: big.NewInt(int64(threshold)),
		client1:   &clientImpl{Client: c1, endpoint: endpoint1},
		client2:   &clientImpl{Client: c2, endpoint: endpoint2},
	}, nil
}

func normalizeLocalhost(endpoint string) string {
	if strings.HasPrefix(endpoint, "localhost") {
		return "http://" + endpoint
	}
	return endpoint
}

func (hc *heightsController) Index(c *gin.Context) {
	var latest1, latest2 block
	err1 := hc.client1.Call(&latest1, "eth_getBlockByNumber", "latest", false)
	err2 := hc.client2.Call(&latest2, "eth_getBlockByNumber", "latest", false)
	merr := multierr.Combine(err1, err2)
	if merr != nil {
		log.Println("Error:", merr)
		c.JSON(502, gin.H{"error": merr.Error()})
		return
	}

	difference := big.NewInt(0)
	difference.Abs(difference.Sub(latest1.Number.ToInt(), latest2.Number.ToInt()))
	resp := gin.H{
		"difference": difference.String(),
		"threshold":  fmt.Sprint(hc.threshold),
		"endpoints": []interface{}{map[string]interface{}{
			"url":    hc.client1.Endpoint(),
			"number": latest1.Number,
		}, map[string]interface{}{
			"url":    hc.client2.Endpoint(),
			"number": latest2.Number,
		}},
	}
	logJSON(resp)
	c.JSON(statusCodeForDifference(hc.threshold, difference), resp)
}

func statusCodeForDifference(threshold, difference *big.Int) int {
	if threshold.Cmp(difference) == -1 {
		return 500
	}
	return 200
}

func logJSON(v gin.H) {
	j, err := json.Marshal(v)
	if err != nil {
		log.Println("Error: unable to marshal event to JSON")
		return
	}
	log.Println(string(j))
}

type client interface {
	Call(result interface{}, method string, args ...interface{}) error
	Endpoint() string
}

type clientImpl struct {
	*rpc.Client
	endpoint string
}

func (c *clientImpl) Endpoint() string {
	return c.endpoint
}

type block struct {
	Number hexutil.Big // public for deserialization by rpc.Client
}

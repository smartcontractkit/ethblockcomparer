package main

import (
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"math/big"
	"net/http"
	"os"
	"strings"

	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/rpc"
	"github.com/gin-gonic/gin"
	"github.com/urfave/cli"
	"go.uber.org/multierr"
)

func main() {
	app := cli.NewApp()
	app.Usage = "CLI for EthBlockComparer: ethblockcomparer <ethereum rpc address 1> <ethereum rpc address 2>"
	app.Version = "1.0.1"
	app.Action = run
	app.Flags = []cli.Flag{
		cli.UintFlag{
			Name:  "threshold, t",
			Usage: "Difference in the block height before returning error",
			Value: 20,
		},
		cli.BoolFlag{
			Name:  "insecure",
			Usage: "If set, skips verification of the server's certificate chain and host name (useful for self-signed certs)",
		},
	}

	if err := app.Run(os.Args); err != nil {
		log.Panic(err)
	}
}

func run(c *cli.Context) error {
	if c.Bool("insecure") {
		http.DefaultTransport.(*http.Transport).TLSClientConfig = &tls.Config{InsecureSkipVerify: true}
	}
	if c.NArg() != 2 {
		return errors.New("must pass the correct number of command line arguments, see `help` for more info")
	}
	endpoint1 := c.Args().Get(0)
	endpoint2 := c.Args().Get(1)
	threshold := c.Uint("threshold")
	r, err := createRouter(endpoint1, endpoint2, threshold)
	if err != nil {
		return err
	}

	log.Print("Comparing block heights from ", endpoint1, " and ", endpoint2, ", erroring when difference is greater than ", threshold)
	if err := r.Run(); err != nil {
		return err
	}
	return nil
}

func createRouter(p1, p2 string, threshold uint) (*gin.Engine, error) {
	r := gin.Default()
	hc, err := newHeightsController(p1, p2, threshold)
	if err != nil {
		return nil, err
	}
	r.GET("/heights", hc.Index)
	return r, err
}

type heightsController struct {
	threshold uint
	client1   client
	client2   client
}

func newHeightsController(endpoint1, endpoint2 string, threshold uint) (*heightsController, error) {
	c1, err1 := rpc.Dial(normalizeLocalhost(endpoint1))
	c2, err2 := rpc.Dial(normalizeLocalhost(endpoint2))
	merr := multierr.Combine(err1, err2)
	if merr != nil {
		return nil, merr
	}
	return &heightsController{
		threshold: threshold,
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
		c.AbortWithError(http.StatusBadGateway, merr)
		return
	}

	difference := calculateDifference(latest1, latest2)
	resp := hc.generateResponse(latest1, latest2, difference)
	logJSON(resp)
	c.JSON(statusCodeForDifference(hc.threshold, difference), resp)
}

func (hc *heightsController) generateResponse(latest1, latest2 block, difference *big.Int) gin.H {
	return gin.H{
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
}

func calculateDifference(latest1, latest2 block) *big.Int {
	difference := big.NewInt(0)
	return difference.Abs(difference.Sub(latest1.Number.ToInt(), latest2.Number.ToInt()))
}

func statusCodeForDifference(threshold uint, difference *big.Int) int {
	bigThresh := big.NewInt(0).SetUint64(uint64(threshold))
	if bigThresh.Cmp(difference) == -1 {
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

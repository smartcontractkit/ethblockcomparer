package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"math/big"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/gin-gonic/gin"
	"github.com/golang/mock/gomock"
	"github.com/smartcontractkit/ethblockcomparer/internal/mocks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCreateRouter_Integration(t *testing.T) {
	threshold := uint(2)
	fakeEth1, cleanup1 := newFakeEthNode("0x1")
	defer cleanup1()
	fakeEth2, cleanup2 := newFakeEthNode("0x2")
	defer cleanup2()

	r, err := createRouter(fakeEth1.URL, fakeEth2.URL, threshold)
	require.NoError(t, err)
	server := httptest.NewServer(r)
	defer server.Close()

	resp, err := http.Get(server.URL + "/heights")
	require.NoError(t, err)
	require.Equal(t, "200 OK", resp.Status)

	defer resp.Body.Close()
	b, err := ioutil.ReadAll(resp.Body)
	require.NoError(t, err)

	j := map[string]interface{}{}
	err = json.Unmarshal(b, &j)
	require.NoError(t, err)
	assert.Equal(t, "1", j["difference"])
	assert.Equal(t, "2", j["threshold"])
}

func TestCreateRouter_Error(t *testing.T) {
	tests := []struct {
		name                 string
		endpoint1, endpoint2 string
		threshold            uint
	}{
		{"bad input", "12gibberish", "http://10.180.0.2:8545", 2},
		// More specific edge cases are covered in TestNewHeightsController
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := createRouter(test.endpoint1, test.endpoint2, test.threshold)
			require.Error(t, err)
		})
	}
}

func TestNewHeightsController(t *testing.T) {
	tests := []struct {
		name                 string
		endpoint1, endpoint2 string
		threshold            uint
		wantError            bool
	}{
		{"empty endpoint1", "", "http://10.180.0.2:8545", 2, true},
		{"empty endpoint2", "http://10.180.0.2:8545", "", 2, true},
		{"bad endpoint1", "12gibberish", "http://10.180.0.2:8545", 2, true},
		{"bad endpoint2", "http://10.180.0.2:8545", "12gibberish", 2, true},
		{"good input", "http://10.180.0.2", "http://172.16.0.2:8545", 2, false},
		{"localhost", "localhost:1234", "http://10.180.0.2:8545", 2, false},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			hc, err := newHeightsController(test.endpoint1, test.endpoint2, test.threshold)
			if test.wantError {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Equal(t, test.endpoint1, hc.client1.Endpoint())
				assert.Equal(t, test.endpoint2, hc.client2.Endpoint())
				assert.Equal(t, uint(2), hc.threshold)
			}
		})
	}
}

func TestHeightsController_Index(t *testing.T) {
	threshold := uint(2)
	tests := []struct {
		name               string
		factory1, factory2 func(*gomock.Controller) *mocks.Mockclient
		status             int
	}{
		{"bad client 1", errorClient, goodClient, 502},
		{"bad client 2", goodClient, errorClient, 502},
		{"good clients", goodClient, goodClient, 200},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()
			mockClient1 := test.factory1(ctrl)
			mockClient2 := test.factory2(ctrl)

			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			hc := heightsController{
				threshold: threshold,
				client1:   mockClient1,
				client2:   mockClient2,
			}

			hc.Index(c)
			require.Equal(t, test.status, c.Writer.Status())
		})
	}
}

func TestHeightsController_GenerateResponse(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	hc := heightsController{
		threshold: 2,
		client1:   goodClient(ctrl),
		client2:   goodClient(ctrl),
	}

	block1 := block{Number: hexutil.Big(*big.NewInt(100))}
	block2 := block{Number: hexutil.Big(*big.NewInt(99))}
	diff := calculateDifference(block1, block2)
	actual := hc.generateResponse(block1, block2, diff)
	expectation := gin.H{
		"difference": "1",
		"threshold":  "2",
		"endpoints": []interface{}{map[string]interface{}{
			"url":    hc.client1.Endpoint(),
			"number": block1.Number,
		}, map[string]interface{}{
			"url":    hc.client2.Endpoint(),
			"number": block2.Number,
		}},
	}

	require.Equal(t, expectation, actual)
}

func TestCalculateDifference(t *testing.T) {
	tests := []struct {
		name               string
		n1, n2, difference int64
	}{
		{"good", 2, 1, 1},
		{"good abs", 1, 2, 1},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			block1 := block{Number: hexutil.Big(*big.NewInt(test.n1))}
			block2 := block{Number: hexutil.Big(*big.NewInt(test.n2))}
			diff := calculateDifference(block1, block2)
			require.Equal(t, big.NewInt(test.difference), diff)
		})
	}
}

func TestStatusCodeForDifference(t *testing.T) {
	tests := []struct {
		name        string
		threshold   uint
		difference  *big.Int
		expectation int
	}{
		{"inside", 2, big.NewInt(1), 200},
		{"border", 2, big.NewInt(2), 200},
		{"outside", 2, big.NewInt(3), 500},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			actual := statusCodeForDifference(test.threshold, test.difference)
			require.Equal(t, test.expectation, actual)
		})
	}
}

func goodClient(ctrl *gomock.Controller) *mocks.Mockclient {
	mc := mocks.NewMockclient(ctrl)
	mc.EXPECT().Call(gomock.Any(), gomock.Any(), gomock.Any()).AnyTimes()
	mc.EXPECT().Endpoint().Return("goodClient.com").AnyTimes()
	return mc
}

func errorClient(ctrl *gomock.Controller) *mocks.Mockclient {
	mc := mocks.NewMockclient(ctrl)
	mc.EXPECT().Call(gomock.Any(), gomock.Any(), gomock.Any()).Return(errors.New("errorClient"))
	return mc
}

func newFakeEthNode(blockHeight string) (*httptest.Server, func()) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		output := fmt.Sprintf(`{"id":1, "jsonrpc":"2.0", "result":{"number":"%s"}}`, blockHeight)
		w.Write([]byte(output))
	})
	server := httptest.NewServer(handler)
	cleanup := func() { server.Close() }
	return server, cleanup
}

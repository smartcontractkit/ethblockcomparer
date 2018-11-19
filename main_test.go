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

	"github.com/gin-gonic/gin"
	"github.com/golang/mock/gomock"
	"github.com/smartcontractkit/ethblockcomparer/internal/mocks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCreateRouter_Integration(t *testing.T) {
	threshold := big.NewInt(2)
	fakeEth1, cleanup1 := newFakeEthNode("0x1")
	defer cleanup1()
	fakeEth2, cleanup2 := newFakeEthNode("0x2")
	defer cleanup2()

	r, err := createRouter(fakeEth1.URL, fakeEth2.URL, threshold.String())
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
		name                            string
		endpoint1, endpoint2, threshold string
	}{
		{"bad input", "12gibberish", "http://10.180.0.2:8545", "2"},
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
		name                            string
		endpoint1, endpoint2, threshold string
		wantError                       bool
	}{
		{"empty endpoint1", "", "http://10.180.0.2:8545", "2", true},
		{"empty endpoint2", "http://10.180.0.2:8545", "", "2", true},
		{"bad endpoint1", "12gibberish", "http://10.180.0.2:8545", "2", true},
		{"bad endpoint2", "http://10.180.0.2:8545", "12gibberish", "2", true},
		{"negative threshold", "http://10.180.0.2", "http://10.180.0.2:8545", "-2", true},
		{"bad threshold", "http://10.180.0.2", "http://10.180.0.2:8545", "ninja", true},
		{"empty threshold", "http://10.180.0.2", "http://10.180.0.2:8545", "", true},
		{"good input", "http://10.180.0.2", "http://172.16.0.2:8545", "2", false},
		{"localhost", "localhost:1234", "http://10.180.0.2:8545", "2", false},
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
				expectation, _ := big.NewInt(0).SetString(test.threshold, 10)
				assert.Equal(t, expectation, hc.threshold)
			}
		})
	}
}

func TestHeightsController_Index(t *testing.T) {
	threshold := big.NewInt(2)
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

func TestStatusCodeForDifference(t *testing.T) {
	tests := []struct {
		name                  string
		threshold, difference *big.Int
		expectation           int
	}{
		{"inside", big.NewInt(2), big.NewInt(1), 200},
		{"border", big.NewInt(2), big.NewInt(2), 200},
		{"outside", big.NewInt(2), big.NewInt(3), 500},
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
	mc.EXPECT().Call(gomock.Any(), gomock.Any(), gomock.Any())
	mc.EXPECT().Endpoint().AnyTimes()
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

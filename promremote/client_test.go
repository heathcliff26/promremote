package promremote

import (
	"bytes"
	"fmt"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewWriteClient(t *testing.T) {
	tMatrix := []struct {
		Name, Endpoint string
		Registry       *prometheus.Registry
		Error          string
	}{
		{"MissingEndpoint", "", prometheus.NewRegistry(), ErrMissingEndpoint{}.Error()},
		{"MissingRegistry", "test-endpoint", nil, ErrMissingRegistry{}.Error()},
		{"CreateRemoteAPIError", "http://%", prometheus.NewRegistry(), NewErrFailedToCreateRemoteAPI(fmt.Errorf("")).Error()},
	}

	for _, tCase := range tMatrix {
		t.Run(tCase.Name, func(t *testing.T) {
			c, err := NewWriteClient(tCase.Endpoint, tCase.Registry)

			require := require.New(t)

			require.ErrorContains(err, tCase.Error, "Should return the correct error")
			require.Nil(c, "Should not return a client")
		})
	}

	t.Run("Success", func(t *testing.T) {
		res, err := NewWriteClient("test-endpoint", prometheus.NewRegistry())

		c := res.(*client)

		assert := assert.New(t)
		require := require.New(t)

		require.NoError(err, "should not return an error")
		require.NotEmpty(c, "Should return a client")

		assert.Equal(defaultJobName, c.job, "Should have default job name")
		assert.Equal(getHostname(), c.instance, "Should have instance")

		require.NotNil(c.client, "Should have http client")
		assert.Equal(httpClientTimeout, c.client.Timeout, "Should have timeout set")
		assert.Nil(c.client.Transport, "Should not have a transport")
	})
}

func TestClientRegistry(t *testing.T) {
	c, _ := NewWriteClient("test-endpoint", prometheus.NewRegistry())
	var cNil *client = nil

	assert := assert.New(t)

	res := c.Registry()
	assert.NotEmpty(res)
	res = cNil.Registry()
	assert.Empty(res)
	assert.NotPanics(func() {
		assert.Nil(cNil.Registry(), "Should return no registry")
	}, "Should not panic when called on nil client")
}

func TestClientWithBasicAuth(t *testing.T) {
	tMatrix := []struct {
		Username, Password string
		ShouldError        bool
	}{
		{"testuser", "password", false},
		{"testuser", "", true},
		{"", "password", true},
		{"", "", true},
	}

	assert := assert.New(t)
	require := require.New(t)

	for _, tCase := range tMatrix {
		c, err := NewWriteClient("test-endpoint", prometheus.NewRegistry(), WithBasicAuth(tCase.Username, tCase.Password))
		if tCase.ShouldError {
			assert.Nil(c, "Should not return a client")
			assert.ErrorContains(err, "Need both username and password, at least one of them is empty", "Should return error")
		} else {
			require.NotNil(c, "Should return a client")
			require.NoError(err, "Should not return an error")
			httpClient := c.(*client).client
			require.IsType(&basicAuthRoundTripper{}, httpClient.Transport, "HTTP client should have basic auth transport")
			assert.Equal(httpClientTimeout, httpClient.Timeout, "Should have timeout set")
		}
	}
}

func TestWithInstanceLabel(t *testing.T) {
	require := require.New(t)

	c, err := NewWriteClient("test-endpoint", prometheus.NewRegistry(), WithInstanceLabel(""))
	require.ErrorContains(err, ErrMissingInstance{}.Error(), "Should return an error with empty instance label")
	require.Nil(c, "Should not return a client with empty instance label")

	c, err = NewWriteClient("test-endpoint", prometheus.NewRegistry(), WithInstanceLabel("testinstance"))
	require.NoError(err, "Should not return an error with valid instance label")
	require.NotNil(c, "Should return a client with valid instance label")
	require.Equal("testinstance", c.(*client).instance, "Should have correct instance label")
}

func TestWithJobLabel(t *testing.T) {
	require := require.New(t)

	c, err := NewWriteClient("test-endpoint", prometheus.NewRegistry(), WithJobLabel(""))
	require.ErrorContains(err, ErrMissingJob{}.Error(), "Should return an error with empty job label")
	require.Nil(c, "Should not return a client with empty job label")

	c, err = NewWriteClient("test-endpoint", prometheus.NewRegistry(), WithJobLabel("testjob"))
	require.NoError(err, "Should not return an error with valid job label")
	require.NotNil(c, "Should return a client with valid job label")
	require.Equal("testjob", c.(*client).job, "Should have correct job label")
}

func TestCollect(t *testing.T) {
	reg := prometheus.NewRegistry()
	reg.MustRegister(collectors.NewBuildInfoCollector())

	c, _ := NewWriteClient("testendpoint", reg)

	assert := assert.New(t)
	require := require.New(t)

	req, err := c.(*client).collect()

	require.NoError(err, "Should collect metrics without error")
	require.NotEmpty(req, "Should return a remote write request")
	require.NotEmpty(req.Timeseries, "Should have collected some metrics")

	assert.NotEmpty(req.Symbols, "Should have a symbol table")
	ts := req.Timeseries[0]
	assert.NotEmpty(ts.Metadata, "TimeSeries should have metadata")
	assert.NotEmpty(ts.LabelsRefs, "TimeSeries should have label references")
	assert.NotEmpty(ts.Samples, "TimeSeries should have samples")
}

func TestRun(t *testing.T) {
	assert := assert.New(t)

	reg := prometheus.NewRegistry()
	reg.MustRegister(collectors.NewBuildInfoCollector())

	c, _ := NewWriteClient("testendpoint", reg)

	var buf bytes.Buffer
	log.SetOutput(&buf)
	t.Cleanup(func() {
		log.SetOutput(os.Stderr)
	})

	err := c.Run(time.Second)
	assert.NoError(err, "Should not return an error")
	assert.ErrorContains(c.Run(time.Second), ErrClientAlreadyRunning{}.Error(), "Successive Run() calls should return an error")

	<-time.After(time.Second * 2)
	c.Stop()

	assert.Eventually(func() bool {
		return !c.IsRunning()
	}, time.Millisecond*200, time.Millisecond*10, "Client should stop running after Stop() called")

	output := buf.String()
	t.Log("Log output:\n", output)
	assert.Contains(output, "ERROR Failed to send metrics to remote endpoint err=", "Should output error to log and not fail")
}

func TestStop(t *testing.T) {
	assert := assert.New(t)

	reg := prometheus.NewRegistry()
	reg.MustRegister(collectors.NewBuildInfoCollector())

	c, _ := NewWriteClient("testendpoint", reg)

	// Stopping before running should be a no-op
	assert.NotPanics(func() {
		c.Stop()
	}, "Stopping a non-running client should not panic")

	assert.NoError(c.Run(time.Second), "Should start the client")

	c.Stop()
	assert.Eventually(func() bool {
		return !c.IsRunning()
	}, time.Millisecond*200, time.Millisecond*10, "Client should stop running after Stop() called")

	// Stopping again should be a no-op
	assert.NotPanics(func() {
		c.Stop()
	}, "Stopping a stopped client should not panic")
}

func TestRemoteRequests(t *testing.T) {
	t.Run("BasicAuth", func(t *testing.T) {
		assert := assert.New(t)

		reg := prometheus.NewRegistry()
		reg.MustRegister(collectors.NewBuildInfoCollector())

		called := make(chan struct{})

		server := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
			assert.Equal("Basic dGVzdHVzZXI6dGVzdHBhc3N3b3Jk", req.Header.Get("Authorization"), "Should have correct basic auth header")
			_, _ = rw.Write(nil)
			close(called)
		}))
		t.Cleanup(server.Close)

		c, _ := NewWriteClient(server.URL, reg, WithBasicAuth("testuser", "testpassword"))

		assert.NoError(c.Run(time.Minute), "Should run client without error")

		assert.Eventually(func() bool {
			select {
			case <-called:
				return true
			default:
				return false
			}
		}, time.Millisecond*200, time.Millisecond*10, "Client should have called server")

		c.Stop()
		assert.Eventually(func() bool {
			return !c.IsRunning()
		}, time.Millisecond*200, time.Millisecond*10, "Client should stop running after Stop() called")
	})
	t.Run("WithoutAuth", func(t *testing.T) {
		assert := assert.New(t)

		reg := prometheus.NewRegistry()
		reg.MustRegister(collectors.NewBuildInfoCollector())

		called := make(chan struct{})

		server := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
			assert.Empty(req.Header.Get("Authorization"), "Should not have basic auth header")
			_, _ = rw.Write(nil)
			close(called)
		}))
		t.Cleanup(server.Close)

		c, _ := NewWriteClient(server.URL, reg)

		assert.NoError(c.Run(time.Minute), "Should run client without error")

		assert.Eventually(func() bool {
			select {
			case <-called:
				return true
			default:
				return false
			}
		}, time.Millisecond*200, time.Millisecond*10, "Client should have called server")

		c.Stop()
		assert.Eventually(func() bool {
			return !c.IsRunning()
		}, time.Millisecond*200, time.Millisecond*10, "Client should stop running after Stop() called")
	})
}

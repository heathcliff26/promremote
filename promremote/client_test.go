package promremote

import (
	"bytes"
	"log"
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
		Name, Endpoint, Instance, Job string
		Registry                      *prometheus.Registry
		Error                         string
	}{
		{"MissingEndpoint", "", "testinstance", "testjob", prometheus.NewRegistry(), "No endpoint for prometheus remote_write provided"},
		{"MissingInstance", "test-endpoint", "", "testjob", prometheus.NewRegistry(), "No instance name provided"},
		{"MissingJob", "test-endpoint", "testinstance", "", prometheus.NewRegistry(), "No job name provided"},
		{"MissingRegistry", "test-endpoint", "testinstance", "testjob", nil, "No prometheus registry provided"},
	}

	for _, tCase := range tMatrix {
		t.Run(tCase.Name, func(t *testing.T) {
			c, err := NewWriteClient(tCase.Endpoint, tCase.Instance, tCase.Job, tCase.Registry)

			require := require.New(t)

			require.Nil(c, "Should not return a client")
			require.ErrorContains(err, tCase.Error, "Should return the correct error")
		})
	}

	t.Run("Success", func(t *testing.T) {
		c, err := NewWriteClient("test-endpoint", "testinstance", "testjob", prometheus.NewRegistry())

		assert := assert.New(t)

		assert.NoError(err, "should not return an error")
		assert.NotEmpty(c, "Should return a client")
	})
}

func TestClientGet(t *testing.T) {
	c, _ := NewWriteClient("test-endpoint", "test", "test", prometheus.NewRegistry())
	var cNil *Client = nil
	t.Run("Endpoint", func(t *testing.T) {
		assert := assert.New(t)

		res := c.Endpoint()
		assert.NotEmpty(res, "Should return endpoint")
		assert.NotPanics(func() {
			assert.Empty(cNil.Endpoint(), "Should return an empty string")
		}, "Should not panic when called on nil client")
	})

	t.Run("Registry", func(t *testing.T) {
		assert := assert.New(t)

		res := c.Registry()
		assert.NotEmpty(res)
		res = cNil.Registry()
		assert.Empty(res)
		assert.NotPanics(func() {
			assert.Nil(cNil.Registry(), "Should return no registry")
		}, "Should not panic when called on nil client")
	})
}

func TestClientSetBasicAuth(t *testing.T) {
	c, _ := NewWriteClient("test-endpoint", "test", "test", prometheus.NewRegistry())

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

	for _, tCase := range tMatrix {
		err := c.SetBasicAuth(tCase.Username, tCase.Password)
		if tCase.ShouldError {
			assert.ErrorContains(err, "Need both username and password, at least one of them is empty", "Should return error")
		} else {
			assert.NoError(err, "Should not return an error")
			assert.Equal(tCase.Username, c.username, "Username should be set")
			assert.Equal(tCase.Password, c.password, "Password should be set")
		}
	}
}

func TestCollect(t *testing.T) {
	reg := prometheus.NewRegistry()
	reg.MustRegister(collectors.NewBuildInfoCollector())

	c, _ := NewWriteClient("testendpoint", "test", "test", reg)

	assert := assert.New(t)
	require := require.New(t)

	req, err := c.collect()

	require.NoError(err, "Should collect metrics without error")
	require.NotEmpty(req, "Should return a remote write request")
	require.NotEmpty(req.Timeseries, "Should have collected some metrics")

	assert.NotEmpty(req.Symbols, "Should have a symbol table")
	ts := req.Timeseries[0]
	assert.NotEmpty(ts.Metadata, "TimeSeries should have metadata")
	assert.NotEmpty(ts.LabelsRefs, "TimeSeries should have label references")
	assert.NotEmpty(ts.Samples, "TimeSeries should have samples")
}

func TestUrl(t *testing.T) {
	t.Run("BasicURL", func(t *testing.T) {
		assert := assert.New(t)

		c := &Client{
			endpoint: "http://prometheus.example.com:1234/test/write",
		}
		url, err := c.url()

		assert.NoError(err, "Should not return an error")
		assert.Equal(c.Endpoint(), url.String(), "Should return correct URL")
	})
	t.Run("WithBasicAuth", func(t *testing.T) {
		assert := assert.New(t)

		c := &Client{
			endpoint: "http://prometheus.example.com:1234/test/write",
			username: "testuser",
			password: "testpassword",
		}
		url, err := c.url()

		assert.NoError(err, "Should not return an error")
		assert.Equal("http://testuser:testpassword@prometheus.example.com:1234/test/write", url.String(), "Should return correct URL")
	})
	t.Run("ParseError", func(t *testing.T) {
		assert := assert.New(t)

		c := &Client{
			endpoint: "%",
		}
		url, err := c.url()

		assert.Error(err, "Should return an error")
		assert.Nil(url, "Should not return a URL")
	})
}

func TestRun(t *testing.T) {
	assert := assert.New(t)

	reg := prometheus.NewRegistry()
	reg.MustRegister(collectors.NewBuildInfoCollector())

	c, _ := NewWriteClient("testendpoint", "test", "test", reg)

	var buf bytes.Buffer
	log.SetOutput(&buf)

	quit := make(chan bool)
	err := c.Run(time.Second, quit)
	assert.NoError(err, "Should not return an error")

	<-time.After(time.Second * 2)
	quit <- true

	log.SetOutput(os.Stderr)

	output := buf.String()
	t.Log(output)
	assert.Contains(output, "ERROR Failed to send metrics to remote endpoint err=", "Should output error to log and not fail")
	assert.Contains(output, "INFO Received stop signal, shutting down remote_write client", "Should shutdown cleanly")
}

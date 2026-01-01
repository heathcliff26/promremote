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
		{"MissingEndpoint", "", "testinstance", "testjob", prometheus.NewRegistry(), ErrMissingEndpoint{}.Error()},
		{"MissingInstance", "test-endpoint", "", "testjob", prometheus.NewRegistry(), ErrMissingInstance{}.Error()},
		{"MissingJob", "test-endpoint", "testinstance", "", prometheus.NewRegistry(), ErrMissingJob{}.Error()},
		{"MissingRegistry", "test-endpoint", "testinstance", "testjob", nil, ErrMissingRegistry{}.Error()},
	}

	for _, tCase := range tMatrix {
		t.Run(tCase.Name, func(t *testing.T) {
			c, err := NewWriteClient(tCase.Endpoint, tCase.Instance, tCase.Job, tCase.Registry)

			require := require.New(t)

			require.ErrorContains(err, tCase.Error, "Should return the correct error")
			require.Nil(c, "Should not return a client")
		})
	}

	t.Run("Success", func(t *testing.T) {
		c, err := NewWriteClient("test-endpoint", "testinstance", "testjob", prometheus.NewRegistry())

		assert := assert.New(t)

		assert.NoError(err, "should not return an error")
		assert.NotEmpty(c, "Should return a client")
	})
}

func TestClientRegistry(t *testing.T) {
	c, _ := NewWriteClient("test-endpoint", "test", "test", prometheus.NewRegistry())
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
		c, err := NewWriteClient("test-endpoint", "test", "test", prometheus.NewRegistry(), WithBasicAuth(tCase.Username, tCase.Password))
		if tCase.ShouldError {
			assert.Nil(c, "Should not return a client")
			assert.ErrorContains(err, "Need both username and password, at least one of them is empty", "Should return error")
		} else {
			require.NotNil(c, "Should return a client")
			require.NoError(err, "Should not return an error")
			client := c.(*client)
			assert.Equal(tCase.Username, client.username, "Username should be set")
			assert.Equal(tCase.Password, client.password, "Password should be set")
		}
	}
}

func TestCollect(t *testing.T) {
	reg := prometheus.NewRegistry()
	reg.MustRegister(collectors.NewBuildInfoCollector())

	c, _ := NewWriteClient("testendpoint", "test", "test", reg)

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

func TestUrl(t *testing.T) {
	t.Run("BasicURL", func(t *testing.T) {
		assert := assert.New(t)

		c := &client{
			endpoint: "http://prometheus.example.com:1234/test/write",
		}
		url, err := c.url()

		assert.NoError(err, "Should not return an error")
		assert.Equal(c.endpoint, url.String(), "Should return correct URL")
	})
	t.Run("WithBasicAuth", func(t *testing.T) {
		assert := assert.New(t)

		c := &client{
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

		c := &client{
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

	c, _ := NewWriteClient("testendpoint", "test", "test", reg)

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

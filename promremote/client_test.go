package promremote

import (
	"bytes"
	"encoding/base64"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/prometheus/prompb"
	"github.com/stretchr/testify/assert"
)

func TestNewWriteClient(t *testing.T) {
	tMatrix := []struct {
		Name, Endpoint, Instance, Job string
		Registry                      *prometheus.Registry
		Error                         string
	}{
		{"MissingEndpoint", "", "testinstance", "testjob", prometheus.NewRegistry(), "promremote.ErrMissingEndpoint"},
		{"MissingInstance", "test-endpoint", "", "testjob", prometheus.NewRegistry(), "promremote.ErrMissingInstance"},
		{"MissingJob", "test-endpoint", "testinstance", "", prometheus.NewRegistry(), "promremote.ErrMissingJob"},
		{"MissingRegistry", "test-endpoint", "testinstance", "testjob", nil, "promremote.ErrMissingRegistry"},
	}

	for _, tCase := range tMatrix {
		t.Run(tCase.Name, func(t *testing.T) {
			c, err := NewWriteClient(tCase.Endpoint, tCase.Instance, tCase.Job, tCase.Registry)

			assert := assert.New(t)

			assert.Nil(c)
			if !assert.Error(err) {
				t.Fatal("Did not receive an error")
			}
			if !assert.Equal(tCase.Error, reflect.TypeOf(err).String()) {
				t.Fatalf("Received invalid error: %v", err)
			}
		})
	}

	t.Run("Success", func(t *testing.T) {
		c, err := NewWriteClient("test-endpoint", "testinstance", "testjob", prometheus.NewRegistry())

		assert := assert.New(t)

		assert.Nil(err)
		assert.NotEmpty(c)
	})
}

func TestClientGet(t *testing.T) {
	c, _ := NewWriteClient("test-endpoint", "test", "test", prometheus.NewRegistry())
	var cNil *Client = nil
	t.Run("Endpoint", func(t *testing.T) {
		assert := assert.New(t)

		res := c.Endpoint()
		assert.NotEmpty(res)
		res = cNil.Endpoint()
		assert.Empty(res)
	})

	t.Run("Registry", func(t *testing.T) {
		assert := assert.New(t)

		res := c.Registry()
		assert.NotEmpty(res)
		res = cNil.Registry()
		assert.Empty(res)
	})
}

func TestClientSetBasicAuth(t *testing.T) {
	c, _ := NewWriteClient("test-endpoint", "test", "test", prometheus.NewRegistry())

	tMatrix := []struct {
		Username, Password string
		Error              error
	}{
		{"testuser", "password", nil},
		{"testuser", "", ErrMissingAuthCredentials{}},
		{"", "password", ErrMissingAuthCredentials{}},
		{"", "", ErrMissingAuthCredentials{}},
	}

	assert := assert.New(t)

	for _, tCase := range tMatrix {
		err := c.SetBasicAuth(tCase.Username, tCase.Password)
		if tCase.Error == nil {
			assert.Nil(err)
			assert.Equal(tCase.Username, c.username)
			assert.Equal(tCase.Password, c.password)
		} else {
			assert.Equal(tCase.Error, err)
		}
	}
}

func TestPost(t *testing.T) {
	tMatrix := []struct {
		Name   string
		Client *Client
		Auth   bool
	}{
		{
			Name:   "WithoutAuth",
			Client: &Client{},
			Auth:   false,
		},
		{
			Name: "WithAuth",
			Client: &Client{
				username: "testuser",
				password: "password",
			},
			Auth: true,
		},
	}

	for _, tCase := range tMatrix {
		t.Run(tCase.Name, func(t *testing.T) {
			assert := assert.New(t)

			server := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
				assert.Equal(http.MethodPost, req.Method)

				assert.Equal("snappy", req.Header.Get("Content-Encoding"))
				assert.Equal("application/x-protobuf", req.Header.Get("Content-Type"))
				assert.Equal("0.1.0", req.Header.Get("X-Prometheus-Remote-Read-Version"))
				if tCase.Auth {
					auth := req.Header.Get("Authorization")
					expectedAuth := "Basic " + base64.StdEncoding.EncodeToString([]byte(tCase.Client.username+":"+tCase.Client.password))
					assert.Equal(expectedAuth, auth)
				}

				_, _ = rw.Write([]byte("Success"))
			}))
			defer server.Close()

			tCase.Client.endpoint = server.URL
			err := tCase.Client.post([]prompb.TimeSeries{})
			assert.Nil(err)
		})
	}
}

func TestCollect(t *testing.T) {
	reg := prometheus.NewRegistry()
	reg.MustRegister(collectors.NewBuildInfoCollector())

	c, _ := NewWriteClient("testendpoint", "test", "test", reg)

	assert := assert.New(t)

	ts, err := c.collect()

	assert.Nil(err)
	assert.NotEmpty(ts)
}

func TestRun(t *testing.T) {
	assert := assert.New(t)

	reg := prometheus.NewRegistry()
	reg.MustRegister(collectors.NewBuildInfoCollector())

	c, _ := NewWriteClient("testendpoint", "test", "test", reg)

	var buf bytes.Buffer
	log.SetOutput(&buf)

	quit := make(chan bool)
	c.Run(time.Second, quit)

	<-time.After(time.Second * 2)
	quit <- true

	log.SetOutput(os.Stderr)

	output := buf.String()
	t.Log(output)
	assert.Contains(output, "ERROR Failed to send metrics to remote endpoint err=", "Should output error to log and not fail")
	assert.Contains(output, "INFO Received stop signal, shutting down remote_write client", "Should shutdown cleanly")
}

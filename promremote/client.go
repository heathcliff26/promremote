package promremote

import (
	"context"
	"fmt"
	"log/slog"
	"net/url"
	"regexp"
	"slices"
	"time"

	"github.com/prometheus/client_golang/exp/api/remote"
	writev2 "github.com/prometheus/client_golang/exp/api/remote/genproto/v2"
	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
)

var (
	fqNameRegex = regexp.MustCompile("fqName: \"([a-zA-Z_:][a-zA-Z0-9_:]*)\"")
	helpRegex   = regexp.MustCompile("help: \"([^\"]*)\"")
)

type Client struct {
	endpoint string
	instance string
	job      string
	username string
	password string
	registry *prometheus.Registry
}

func NewWriteClient(endpoint, instance, job string, reg *prometheus.Registry) (*Client, error) {
	if endpoint == "" {
		return nil, ErrMissingEndpoint{}
	}
	if instance == "" {
		return nil, ErrMissingInstance{}
	}
	if job == "" {
		return nil, ErrMissingJob{}
	}
	if reg == nil {
		return nil, ErrMissingRegistry{}
	}
	return &Client{
		endpoint: endpoint,
		instance: instance,
		job:      job,
		registry: reg,
	}, nil
}

func (c *Client) Endpoint() string {
	if c == nil {
		return ""
	}
	return c.endpoint
}

func (c *Client) Registry() *prometheus.Registry {
	if c == nil {
		return nil
	}
	return c.registry
}

// Set credentials needed for basic auth, return error if not provided
func (c *Client) SetBasicAuth(username, password string) error {
	if username == "" || password == "" {
		return ErrMissingAuthCredentials{}
	}
	c.username = username
	c.password = password
	return nil
}

// Collect metrics from registry and convert them to TimeSeries
func (c *Client) collect() (*writev2.Request, error) {
	ch := make(chan prometheus.Metric)
	go func() {
		c.registry.Collect(ch)
		close(ch)
	}()

	res := &writev2.Request{}
	s := writev2.NewSymbolTable()

	for metric := range ch {
		// Extract name of metric
		fqName := fqNameRegex.FindStringSubmatch(metric.Desc().String())
		if len(fqName) < 2 {
			return nil, &ErrInvalidMetricDesc{Desc: metric.Desc().String()}
		}
		helpRef := helpRegex.FindStringSubmatch(metric.Desc().String())
		help := ""
		if len(helpRef) == 2 {
			help = helpRef[1]
		}

		// Convert metric to readable format
		m := &dto.Metric{}
		err := metric.Write(m)
		if err != nil {
			return nil, err
		}

		// Extract labels
		labels := make([]string, 0, 2*(len(m.Label)+3))
		labels = append(labels, "__name__", fqName[1], "instance", c.instance, "job", c.job)
		dropLabels := []string{"__name__", "instance", "job"}
		for _, l := range m.Label {
			if !slices.Contains(dropLabels, l.GetName()) {
				labels = append(labels, l.GetName(), l.GetValue())
			}
		}

		// Extract value and timestamp
		var metricType writev2.Metadata_MetricType
		var value float64
		if m.Counter != nil {
			metricType = writev2.Metadata_METRIC_TYPE_COUNTER
			value = m.Counter.GetValue()
		} else if m.Gauge != nil {
			metricType = writev2.Metadata_METRIC_TYPE_GAUGE
			value = m.Gauge.GetValue()
		} else if m.Untyped != nil {
			metricType = writev2.Metadata_METRIC_TYPE_UNSPECIFIED
			value = m.Untyped.GetValue()
		} else {
			return nil, fmt.Errorf("unknown metric type")
		}

		ts := &writev2.TimeSeries{
			Metadata: &writev2.Metadata{
				Type:    metricType,
				HelpRef: s.Symbolize(help),
			},
			LabelsRefs: s.SymbolizeLabels(labels, nil),
			Samples: []*writev2.Sample{
				{
					Value:     value,
					Timestamp: time.Now().UnixMilli(),
				},
			},
		}

		res.Timeseries = append(res.Timeseries, ts)
	}
	res.Symbols = s.Symbols()
	return res, nil
}

// Return the url of the remote_write endpoint with optional basic auth credentials
func (c *Client) url() (*url.URL, error) {
	parsedURL, err := url.Parse(c.Endpoint())
	if err != nil {
		return nil, err
	}

	if c.username != "" {
		parsedURL.User = url.UserPassword(c.username, c.password)
	}

	return parsedURL, nil
}

// Collect metrics and send them to remote server in interval.
// Does not block main thread execution
func (c *Client) Run(interval time.Duration, quit chan bool) error {
	endpoint, err := c.url()
	if err != nil {
		return err
	}
	// Set an empty path to ensure that our own path is not overridden with the default path.
	// Disable retries by setting MaxRetries to -1.
	rwAPI, err := remote.NewAPI(endpoint.String(), remote.WithAPIPath(""), remote.WithAPIBackoff(remote.BackoffConfig{MaxRetries: -1}))
	if err != nil {
		return NewErrFailedToCreateRemoteAPI(err)
	}

	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		slog.Debug("Starting remote_write client")
		for {
			req, err := c.collect()
			if err != nil {
				slog.Error("Failed to collect metrics for remote_write", "err", err)
			}

			stats, err := rwAPI.Write(context.TODO(), remote.WriteV2MessageType, req)
			if err != nil {
				slog.Error("Failed to send metrics to remote endpoint", "err", err)
			} else {
				slog.Debug("Successfully sent metrics via remote_write", slog.Int("count", stats.AllSamples()), slog.Bool("written", !stats.NoDataWritten()))
			}
			select {
			case <-ticker.C:

			case <-quit:
				slog.Info("Received stop signal, shutting down remote_write client")
				return
			}
		}
	}()

	return nil
}

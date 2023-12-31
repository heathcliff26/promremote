# promremote

promremote is a golang API for pushing metrics collected from [client_golang](https://github.com/prometheus/client_golang) to prometheus via remote_write.

Here is an example usage snippet:
```
rwClient, err := promremote.NewWriteClient(url, "some-instance", "integrations/some-job", reg)
if err != nil {
    slog.Error("Failed to create remote write client", "err", err)
    os.Exit(1)
}
err := rwClient.SetBasicAuth(username, password)
if err != nil {
    slog.Error("Failed to create remote_write client", "err", err)
    os.Exit(1)
}

slog.Info("Starting remote_write client")
rwQuit := make(chan bool)
rwClient.Run(interval, rwQuit)
defer func() {
    rwQuit <- true
    close(rwQuit)
}()
```

This creates a new promremote client. It will then collect metrics from the given registry and push them to prometheus periodically.

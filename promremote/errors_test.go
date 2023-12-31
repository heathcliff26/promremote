package promremote

import (
	"io"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNewErrRemoteWriteFailed(t *testing.T) {
	result := &ErrRemoteWriteFailed{StatusCode: 400, Body: "testresult"}
	r := io.NopCloser(strings.NewReader(result.Body))
	defer r.Close()
	err := NewErrRemoteWriteFailed(400, r)
	assert.Equal(t, result, err)
}

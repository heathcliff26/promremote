package promremote

import (
	"io"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestErrRemoteWriteFailed(t *testing.T) {
	t.Run("New", func(t *testing.T) {
		assert := assert.New(t)

		expectedResult := &ErrRemoteWriteFailed{StatusCode: 400, Body: "testresult"}

		r := io.NopCloser(strings.NewReader(expectedResult.Body))
		defer r.Close()
		result := NewErrRemoteWriteFailed(400, r).(*ErrRemoteWriteFailed)
		assert.Equal(expectedResult, result, "Should read the response body")

		f, err := os.Open("/dev/null")
		if !assert.NoError(err, "Should not fail to open /dev/null") {
			t.FailNow()
		}
		f.Close()
		result = NewErrRemoteWriteFailed(400, f).(*ErrRemoteWriteFailed)
		assert.Contains(result.Body, "file already closed", "Should return error of reader as body")
	})

}

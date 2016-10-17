package gcm

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
)

var data = map[string]string{"k": "v"}
var msg = &Message{Data: data}
var twoRecipients = []string{"1", "2"}
var topic = TopicPrefix + "global"
var success = response{Success: 1, Results: []result{{MessageID: "id"}}}
var fail = response{Failure: 1, Results: []result{{Err: ErrorUnavailable}}}
var partialDeviceGroup = response{Success: 1, Failure: 2, FailedRegistrationIDs: []string{"id1", "id2"}}
var partialMulticast = response{MulticastID: 1, Success: 1, Failure: 1, Results: []result{{MessageID: "id1"}, {Err: ErrorUnavailable}}}

func TestSendWithInvalidAPIKey(t *testing.T) {
	server := startTestServer(t)
	defer server.Close()
	s := NewSender("")
	_, err := s.SendNoRetry(msg, "1")
	assert.EqualError(t, err, "missing API key")
	_, err = s.SendWithRetries(msg, "1", 1)
	assert.EqualError(t, err, "missing API key")
	_, err = s.SendMulticastNoRetry(msg, twoRecipients)
	assert.EqualError(t, err, "missing API key")
	_, err = s.SendMulticastWithRetries(msg, twoRecipients, 1)
	assert.EqualError(t, err, "missing API key")
}

func TestSendWithInvalidMessage(t *testing.T) {
	server := startTestServer(t)
	defer server.Close()
	s := NewSender("test-api-key")
	params := []struct {
		msg *Message
		err string
	}{
		{nil, "message cannot be nil"},
		{&Message{TimeToLive: -1}, "TimeToLive should be non-negative and at most 4 weeks"},
		{&Message{TimeToLive: 2419201}, "TimeToLive should be non-negative and at most 4 weeks"},
	}
	for _, param := range params {
		_, err := s.SendNoRetry(param.msg, "1")
		assert.EqualError(t, err, param.err)
		_, err = s.SendWithRetries(param.msg, "1", 1)
		assert.EqualError(t, err, param.err)
		_, err = s.SendMulticastNoRetry(param.msg, twoRecipients)
		assert.EqualError(t, err, param.err)
		_, err = s.SendMulticastWithRetries(param.msg, twoRecipients, 1)
		assert.EqualError(t, err, param.err)
	}
}

func TestSendWithInvalidRetries(t *testing.T) {
	server := startTestServer(t)
	defer server.Close()
	s := NewSender("test-api-key")
	_, err := s.SendWithRetries(msg, "1", -1)
	assert.EqualError(t, err, "retries cannot be negative")
	_, err = s.SendMulticastWithRetries(msg, twoRecipients, -1)
	assert.EqualError(t, err, "retries cannot be negative")
}

func TestSendWithInvalidRecipients(t *testing.T) {
	server := startTestServer(t)
	defer server.Close()
	s := NewSender("test-api-key")
	_, err := s.SendNoRetry(msg, "")
	assert.EqualError(t, err, "missing recipient(s)")
	_, err = s.SendWithRetries(msg, "", 0)
	assert.EqualError(t, err, "missing recipient(s)")
	_, err = s.SendMulticastNoRetry(msg, nil)
	assert.EqualError(t, err, "missing recipient(s)")
	_, err = s.SendMulticastNoRetry(msg, []string{})
	assert.EqualError(t, err, "missing recipient(s)")
	_, err = s.SendMulticastWithRetries(msg, nil, 0)
	assert.EqualError(t, err, "missing recipient(s)")
	_, err = s.SendMulticastWithRetries(msg, []string{}, 0)
	assert.EqualError(t, err, "missing recipient(s)")
}

func TestSendRetryOk_DueToApiError(t *testing.T) {
	server := startTestServer(t,
		&testResponse{response: &fail},
		&testResponse{response: &success},
	)
	defer server.Close()
	s := NewSender("test-api-key")
	result, err := s.SendWithRetries(msg, "regId", 1)
	assert.NoError(t, err)
	assert.Equal(t, Result{MessageID: "id"}, *result)
}

func TestSendRetryOk_DueToHttpError(t *testing.T) {
	server := startTestServer(t,
		&testResponse{statusCode: http.StatusInternalServerError},
		&testResponse{response: &success},
	)
	defer server.Close()
	s := NewSender("test-api-key")
	result, err := s.SendWithRetries(msg, "regId", 1)
	assert.NoError(t, err)
	assert.Equal(t, Result{MessageID: "id"}, *result)
}

func TestSendRetryFail_DueToExceededRetries(t *testing.T) {
	server := startTestServer(t,
		&testResponse{response: &fail},
		&testResponse{response: &fail},
	)
	defer server.Close()
	s := NewSender("test-api-key")
	result, err := s.SendWithRetries(msg, "regId", 1)
	assert.NoError(t, err)
	assert.Equal(t, Result{Error: ErrorUnavailable}, *result)
}

func TestSendRetryFail_DueToTopicRateExceeded(t *testing.T) {
	server := startTestServer(t, &testResponse{response: &response{Err: ErrorTopicsMessageRateExceeded}})
	defer server.Close()
	s := NewSender("test-api-key")
	result, err := s.SendWithRetries(msg, topic, 1)
	assert.NoError(t, err)
	assert.Equal(t, Result{Error: ErrorTopicsMessageRateExceeded}, *result)
}

func TestSendRetryFail_DueToDeviceGroupPartialFail(t *testing.T) {
	server := startTestServer(t, &testResponse{response: &partialDeviceGroup})
	defer server.Close()
	s := NewSender("test-api-key")
	result, err := s.SendWithRetries(msg, "group", 1)
	assert.NoError(t, err)
	assert.Equal(t, Result{Success: 1, Failure: 2, FailedRegistrationIDs: []string{"id1", "id2"}}, *result)
}

func TestSendRetryError_DueToUnrecoverableHttpError(t *testing.T) {
	server := startTestServer(t, &testResponse{statusCode: http.StatusBadRequest})
	defer server.Close()
	s := NewSender("test-api-key")
	_, err := s.SendWithRetries(msg, "regId", 1)
	assert.EqualError(t, err, "400 error: 400 Bad Request")
}

func TestSendMulticastRetryError_DueToUnrecoverableHttpError(t *testing.T) {
	server := startTestServer(t, &testResponse{statusCode: http.StatusBadRequest})
	defer server.Close()
	s := NewSender("test-api-key")
	_, err := s.SendMulticastWithRetries(msg, twoRecipients, 1)
	assert.EqualError(t, err, "400 error: 400 Bad Request")
}

func TestSendMulticastRetryOk(t *testing.T) {
	server := startTestServer(t,
		&testResponse{response: &partialMulticast},
		&testResponse{response: &response{MulticastID: 2, Success: 1, Results: []result{{MessageID: "id2"}}}},
	)
	defer server.Close()
	s := NewSender("test-api-key")
	result, err := s.SendMulticastWithRetries(msg, twoRecipients, 1)
	assert.NoError(t, err)
	assert.Equal(t, MulticastResult{MulticastID: 1, Success: 2, RetryMulticastIDs: []int64{2}, Results: []Result{{MessageID: "id1"}, {MessageID: "id2"}}}, *result)
}

func TestSendMulticastRetryPartialFail_DueToExceededRetries(t *testing.T) {
	server := startTestServer(t,
		&testResponse{response: &partialMulticast},
		&testResponse{response: &response{MulticastID: 2, Failure: 1, Results: []result{{Err: ErrorUnavailable}}}},
	)
	defer server.Close()
	s := NewSender("test-api-key")
	result, err := s.SendMulticastWithRetries(msg, twoRecipients, 1)
	assert.NoError(t, err)
	assert.Equal(t, MulticastResult{
		MulticastID:       1,
		Success:           1,
		Failure:           1,
		RetryMulticastIDs: []int64{2},
		Results:           []Result{{MessageID: "id1"}, {Error: ErrorUnavailable}},
	}, *result)
}

func TestSendMulticastRetryPartialFail_DueToUnrecoverableError(t *testing.T) {
	server := startTestServer(t,
		&testResponse{response: &partialMulticast},
		&testResponse{statusCode: http.StatusBadRequest},
	)
	defer server.Close()
	s := NewSender("test-api-key")
	result, err := s.SendMulticastWithRetries(msg, twoRecipients, 1)
	assert.NoError(t, err)
	assert.Equal(t, MulticastResult{
		MulticastID: 1,
		Success:     1,
		Failure:     1,
		Results:     []Result{{MessageID: "id1"}, {Error: ErrorUnavailable}},
	}, *result)
}

type testResponse struct {
	statusCode int
	response   *response
}

func startTestServer(t *testing.T, responses ...*testResponse) *httptest.Server {
	i := 0
	handler := func(w http.ResponseWriter, r *http.Request) {
		if i >= len(responses) {
			t.Fatalf("server received %d requests, expected %d", i+1, len(responses))
		}
		resp := responses[i]
		status := resp.statusCode
		if status == 0 || status == http.StatusOK {
			w.Header().Set("Content-Type", "application/json")
			respBytes, _ := json.Marshal(resp.response)
			fmt.Fprint(w, string(respBytes))
		} else {
			w.WriteHeader(status)
		}
		i++
	}
	server := httptest.NewServer(http.HandlerFunc(handler))
	GCMEndpoint = server.URL
	return server
}

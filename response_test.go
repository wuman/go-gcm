package gcm

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestMarshalUnmarshal(t *testing.T) {
	type param struct {
		json string
		resp *response
	}
	params := []param{
		{`{"success":1,"results":[{"message_id":"id"}]}`, &response{Success: 1, Results: []result{{MessageID: "id"}}}},
		// topic message responses
		{`{"message_id":10}`, &response{MessageID: 10}},
		{`{"error":"TopicsMessageRateExceeded"}`, &response{Err: ErrorTopicsMessageRateExceeded}},
		// device group message responses
		{`{"success":2}`, &response{Success: 2, Failure: 0}},
		{`{"success":1,"failure":2,"failed_registration_ids":["id1","id2"]}`, &response{Success: 1, Failure: 2, FailedRegistrationIDs: []string{"id1", "id2"}}},
	}
	for _, param := range params {
		if param.json != "" {
			r := response{}
			assert.NoError(t, json.NewDecoder(strings.NewReader(param.json)).Decode(&r))
			assert.Equal(t, *param.resp, r)
		}
	}
	for _, param := range params {
		if param.resp != nil {
			b, err := json.Marshal(*param.resp)
			assert.NoError(t, err)
			assert.Equal(t, param.json, string(b))
		}
	}

}

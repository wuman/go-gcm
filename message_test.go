package gcm

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestMessageMarshalUnmarshal(t *testing.T) {
	type param struct {
		json string
		msg  *message
		err  error
	}
	params := []param{
		// success cases
		{`{"registration_ids":["1","2"]}`, &message{registrationIds: []string{"1", "2"}}, nil},
		{`{"priority":"normal"}`, &message{Message: Message{Priority: PriorityNormal}}, nil},
		{`{"priority":"high"}`, &message{Message: Message{Priority: PriorityHigh}}, nil},
		{`{"data":{"k":"v"}}`, &message{Message: Message{Data: map[string]string{"k": "v"}}}, nil},
		{`{"notification":{"title":"test"}}`, &message{Message: Message{Notification: &Notification{Title: "test"}}}, nil},
		// unmarshal failure cases
		{`{"priority":"nok"}`, nil, errors.New("priority should be either normal or high, got nok")},
		// marshal failure cases
		{"", &message{Message: Message{Priority: 3}}, errors.New("json: error calling MarshalJSON for type gcm.message: json: error calling MarshalJSON for type gcm.Priority: invalid priority value: 3")},
	}
	for _, param := range params {
		if param.json != "" {
			m := message{}
			err := json.NewDecoder(strings.NewReader(param.json)).Decode(&m)
			if param.err != nil {
				assert.EqualError(t, err, param.err.Error())
			} else {
				assert.NoError(t, err)
				assert.EqualValues(t, *param.msg, m)
			}
		}
	}
	for _, param := range params {
		if param.msg != nil {
			b, err := json.Marshal(*param.msg)
			if param.err != nil {
				assert.EqualError(t, err, param.err.Error())
			} else {
				assert.NoError(t, err)
				assert.Equal(t, param.json, string(b))
			}
		}
	}
}

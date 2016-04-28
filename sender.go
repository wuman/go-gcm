package gcm

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"log"
	"math/rand"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const (
	// endpoint for the GCM connection server owned by Google
	ConnectionServerEndpoint = "https://android.googleapis.com/gcm/send"
	// initial retry intervl in milliseconds for exponential backoff
	BackoffInitialDelay = 1000
	// max backoff period in milliseconds
	MaxBackoffDelay = 1024000
)

// for unit test only
var gcmEndpoint = ConnectionServerEndpoint

type Sender struct {
	ApiKey string
	Client *http.Client
}

func NewSender(apiKey string) *Sender {
	return NewSenderWithHttpClient(apiKey, new(http.Client))
}

func NewSenderWithHttpClient(apiKey string, client *http.Client) *Sender {
	return &Sender{apiKey, client}
}

func checkUnrecoverableErrors(s *Sender, to string, regIds []string, msg *Message, retries int) error {
	// check sender
	if s.ApiKey == "" {
		return fmt.Errorf("missing API key")
	}
	if s.Client == nil {
		s.Client = new(http.Client)
	}
	// check message
	if msg == nil {
		return errors.New("message cannot be nil")
	}
	if msg.TimeToLive < 0 || msg.TimeToLive > 2419200 {
		return errors.New("TimeToLive should be non-negative and at most 4 weeks")
	}
	// check recipients
	if to == "" && (regIds == nil || len(regIds) <= 0) {
		return errors.New("missing recipient(s)")
	}
	// check retries
	if retries < 0 {
		return errors.New("retries cannot be negative")
	}
	return nil
}

type httpError struct {
	statusCode int
	status     string
}

func (e httpError) Error() string {
	return fmt.Sprintf("%d error: %s", e.statusCode, e.status)
}

func (s *Sender) sendRaw(msg *message) (*response, error) {
	if err := checkUnrecoverableErrors(s, msg.to, msg.registrationIds, &msg.Message, 0); err != nil {
		return nil, err
	}

	msgJson, err := json.Marshal(msg)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequest("POST", gcmEndpoint, bytes.NewBuffer(msgJson))
	if err != nil {
		return nil, err
	}
	req.Header.Add("Authorization", fmt.Sprintf("key=%s", s.ApiKey))
	req.Header.Add("Content-Type", "application/json")

	resp, err := s.Client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		// refer to https://goo.gl/nV1Nf6
		// 400: bad json or contains invalid fields
		// 401: sender authentication failure
		// 5xx: GCM connection server internal error (retry later)
		return nil, httpError{resp.StatusCode, resp.Status}
	}

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	response := new(response)
	err = json.Unmarshal(body, response)
	if err != nil {
		log.Printf("failed to unmarshal json: %s", body)
		return nil, err
	}

	return response, nil
}

// SendNoRetry sends a downstream message without retries.  The recipient can
// be one of 3 types: single recipient specified with a registration id,
// recipients subscribed to a topic specified with a topic name, members of a
// device group specified with a notification key.
func (s *Sender) SendNoRetry(msg *Message, to string) (*Result, error) {
	if err := checkUnrecoverableErrors(s, to, nil, msg, 0); err != nil {
		return nil, err
	}
	rawMsg := &message{Message: *msg, to: to}

	resp, err := s.sendRaw(rawMsg)
	if err != nil {
		return nil, err
	}

	result := new(Result)
	if resp.Results != nil { // downstream message
		if len(resp.Results) != 1 {
			return nil, fmt.Errorf("invalid response.results: %v", resp.Results)
		}
		res := resp.Results[0]
		result.MessageId = res.MessageId
		result.CanonicalRegistrationId = res.RegistrationId
		result.Error = res.Err
	} else if strings.HasPrefix(to, TopicPrefix) { // topic message
		if resp.MessageId != 0 {
			result.MessageId = strconv.FormatInt(resp.MessageId, 10)
		} else if resp.Err != "" {
			result.Error = resp.Err
		} else {
			return nil, fmt.Errorf("expected message_id or error, but found: %v", *resp)
		}
	} else { // device group message
		result.Success = resp.Success
		result.Failure = resp.Failure
		result.FailedRegistrationIds = resp.FailedRegistrationIds // partial success
	}

	return result, nil
}

// SendWithRetries sends a downstream message with retries.
func (s *Sender) SendWithRetries(msg *Message, to string, retries int) (result *Result, err error) {
	if err := checkUnrecoverableErrors(s, to, nil, msg, retries); err != nil {
		return nil, err
	}
	attempt, backoff := 0, BackoffInitialDelay
	for {
		attempt++
		result, err = s.SendNoRetry(msg, to)
		// NOTE: partial success for a device group message is considered successful

		tryAgain := false
		if attempt <= retries {
			if result != nil && (result.Error == ErrorUnavailable || result.Error == ErrorInternalServerError) {
				tryAgain = true
			} else if err != nil {
				if httpErr, isHttpErr := err.(httpError); isHttpErr {
					tryAgain = httpErr.statusCode >= http.StatusInternalServerError && httpErr.statusCode < 600
				}
			}
		}

		if tryAgain {
			sleepTime := backoff/2 + rand.Intn(backoff)
			time.Sleep(time.Duration(sleepTime) * time.Millisecond)
			backoff = min(2*backoff, MaxBackoffDelay)
		} else {
			break
		}
	}
	return
}

// SendMulticastNoRetry sends a multicast message to multiple recipients without
// retries.
func (s *Sender) SendMulticastNoRetry(msg *Message, registrationIds []string) (*MulticastResult, error) {
	if err := checkUnrecoverableErrors(s, "", registrationIds, msg, 0); err != nil {
		return nil, err
	}
	rawMsg := &message{Message: *msg, registrationIds: registrationIds}

	resp, err := s.sendRaw(rawMsg)
	if err != nil {
		return nil, err
	}

	result := new(MulticastResult)
	result.Success = resp.Success
	result.Failure = resp.Failure
	result.CanonicalIds = resp.CanonicalIds
	result.MulticastId = resp.MulticastId
	if resp.Results != nil {
		result.Results = make([]Result, len(resp.Results))
		for i, res := range resp.Results {
			result.Results[i] = Result{
				MessageId:               res.MessageId,
				CanonicalRegistrationId: res.RegistrationId,
				Error: res.Err,
			}
		}
	}
	return result, nil
}

// SendMulticastWithRetries sends a multicast message to the GCM connection
// server, retrying with exponential backoff when the server is unavailable.
// Note that only the following error incidents are retried:
//   * 200 + error:Unavailable
//   * 200 + error:InternalServerError
// 5xx HTTP status codes are not retried to keep the code simple.
func (s *Sender) SendMulticastWithRetries(msg *Message, regIds []string, retries int) (*MulticastResult, error) {
	if err := checkUnrecoverableErrors(s, "", regIds, msg, retries); err != nil {
		return nil, err
	}
	rawMsg := &message{Message: *msg, registrationIds: regIds}

	results := make(map[string]result, len(regIds))
	finalResult, backoff, firstResponse := new(MulticastResult), BackoffInitialDelay, true

	for {
		resp, err := s.sendRaw(rawMsg)
		if err != nil {
			if httpErr, isHttpErr := err.(httpError); isHttpErr && httpErr.statusCode >= 500 && httpErr.statusCode < 600 {
				// recoverable error, so continue to retry
			} else if firstResponse {
				// unrecoverable first response
				return nil, err
			} else {
				// NOTE: unrecoverable error but we had partial results previously,
				// so return partial results with nil error.
				break
			}
		}

		var retryRegIds []string
		if resp != nil {
			if resp.MulticastId != 0 {
				if firstResponse {
					finalResult.MulticastId = resp.MulticastId
				} else {
					finalResult.RetryMulticastIds = append(finalResult.RetryMulticastIds, resp.MulticastId)
				}
			}

			retryRegIds = make([]string, 0, resp.Failure)
			for i := range resp.Results {
				regId, result := rawMsg.registrationIds[i], resp.Results[i]
				results[regId] = result
				if result.Err == ErrorUnavailable || result.Err == ErrorInternalServerError {
					retryRegIds = append(retryRegIds, regId)
				}
			}
		} else {
			retryRegIds = make([]string, len(rawMsg.registrationIds))
			for i := range rawMsg.registrationIds {
				retryRegIds[i] = rawMsg.registrationIds[i]
			}
		}

		firstResponse = false
		if retries <= 0 || len(retryRegIds) == 0 {
			break
		}

		rawMsg.registrationIds = retryRegIds
		sleepTime := backoff/2 + rand.Intn(backoff)
		time.Sleep(time.Duration(sleepTime) * time.Millisecond)
		backoff = min(2*backoff, MaxBackoffDelay)
		retries--
	}

	// reconstruct final results
	finalResults := make([]Result, len(regIds))
	for i, regId := range regIds {
		result := results[regId]
		finalResults[i] = Result{
			MessageId:               result.MessageId,
			CanonicalRegistrationId: result.RegistrationId,
			Error: result.Err,
		}
		if result.MessageId != "" {
			finalResult.Success++
			if result.RegistrationId != "" {
				finalResult.CanonicalIds++
			}
		} else {
			finalResult.Failure++
		}
	}
	finalResult.Results = finalResults
	return finalResult, nil
}

func min(x, y int) int {
	if x < y {
		return x
	}
	return y
}

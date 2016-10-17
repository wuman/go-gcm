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
	// ConnectionServerEndpoint defines the endpoint for the GCM connection server owned by Google.
	ConnectionServerEndpoint = "https://android.googleapis.com/gcm/send"
	// FCMServerEndpoint defines the endpoint for the FCM connection server by Firebase.
	FCMServerEndpoint = "https://fcm.googleapis.com/fcm/send"
	// BackoffInitialDelay defines the initial retry interval in milliseconds for exponential backoff.
	BackoffInitialDelay = 1000
	// MaxBackoffDelay defines the max backoff period in milliseconds.
	MaxBackoffDelay = 1024000
)

// GCMEndpoint by default points to the GCM connection server owned by Google,
// but can be otherwise set to a differnet URL if needed (e.g. FCMServerEndpoint).
var GCMEndpoint = ConnectionServerEndpoint

// Sender sends GCM messages to the GCM connection server.
type Sender struct {
	// APIKey specifies the API key.
	APIKey string
	// Client is the http client used for transport.  By default it is just http.Client.
	Client *http.Client
}

// NewSender instantiates a Sender given the API key.
func NewSender(apiKey string) *Sender {
	return NewSenderWithHTTPClient(apiKey, new(http.Client))
}

// NewSenderWithHTTPClient instantiates a Sender given the API key and an http.Client.
func NewSenderWithHTTPClient(apiKey string, client *http.Client) *Sender {
	return &Sender{apiKey, client}
}

func checkUnrecoverableErrors(s *Sender, to string, regIDs []string, msg *Message, retries int) error {
	// check sender
	if s.APIKey == "" {
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
	if to == "" && (regIDs == nil || len(regIDs) <= 0) {
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

	msgJSON, err := json.Marshal(msg)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequest("POST", GCMEndpoint, bytes.NewBuffer(msgJSON))
	if err != nil {
		return nil, err
	}
	req.Header.Add("Authorization", fmt.Sprintf("key=%s", s.APIKey))
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
		result.MessageID = res.MessageID
		result.CanonicalRegistrationID = res.RegistrationID
		result.Error = res.Err
	} else if strings.HasPrefix(to, TopicPrefix) { // topic message
		if resp.MessageID != 0 {
			result.MessageID = strconv.FormatInt(resp.MessageID, 10)
		} else if resp.Err != "" {
			result.Error = resp.Err
		} else {
			return nil, fmt.Errorf("expected message_id or error, but found: %v", *resp)
		}
	} else { // device group message
		result.Success = resp.Success
		result.Failure = resp.Failure
		result.FailedRegistrationIDs = resp.FailedRegistrationIDs // partial success
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
				if httpErr, isHTTPErr := err.(httpError); isHTTPErr {
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
	result.MulticastID = resp.MulticastID
	if resp.Results != nil {
		result.Results = make([]Result, len(resp.Results))
		for i, res := range resp.Results {
			result.Results[i] = Result{
				MessageID:               res.MessageID,
				CanonicalRegistrationID: res.RegistrationID,
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
func (s *Sender) SendMulticastWithRetries(msg *Message, regIDs []string, retries int) (*MulticastResult, error) {
	if err := checkUnrecoverableErrors(s, "", regIDs, msg, retries); err != nil {
		return nil, err
	}
	rawMsg := &message{Message: *msg, registrationIds: regIDs}

	results := make(map[string]result, len(regIDs))
	finalResult, backoff, firstResponse := new(MulticastResult), BackoffInitialDelay, true

	for {
		resp, err := s.sendRaw(rawMsg)
		if err != nil {
			if httpErr, isHTTPErr := err.(httpError); isHTTPErr && httpErr.statusCode >= 500 && httpErr.statusCode < 600 {
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
			if resp.MulticastID != 0 {
				if firstResponse {
					finalResult.MulticastID = resp.MulticastID
				} else {
					finalResult.RetryMulticastIDs = append(finalResult.RetryMulticastIDs, resp.MulticastID)
				}
			}

			retryRegIds = make([]string, 0, resp.Failure)
			for i := range resp.Results {
				regID, result := rawMsg.registrationIds[i], resp.Results[i]
				results[regID] = result
				if result.Err == ErrorUnavailable || result.Err == ErrorInternalServerError {
					retryRegIds = append(retryRegIds, regID)
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
	finalResults := make([]Result, len(regIDs))
	for i, regID := range regIDs {
		result := results[regID]
		finalResults[i] = Result{
			MessageID:               result.MessageID,
			CanonicalRegistrationID: result.RegistrationID,
			Error: result.Err,
		}
		if result.MessageID != "" {
			finalResult.Success++
			if result.RegistrationID != "" {
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

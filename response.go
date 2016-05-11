package gcm

// reference: https://developers.google.com/cloud-messaging/http-server-ref

// response specifies the downstream HTTP message response body in JSON format.
// Refer to https://goo.gl/XqsQ6w.
type response struct {
	// unique ID identifying the multicast message
	MulticastID int64 `json:"multicast_id,omitempty"`
	// number of messages that were processed without an error
	Success int `json:"success,omitempty"`
	// number of messages that could not be processed
	Failure int `json:"failure,omitempty"`
	// number of results containing a canonical registration token
	CanonicalIds int      `json:"canonical_ids,omitempty"`
	Results      []result `json:"results,omitempty"`
	// topic messages only, see https://goo.gl/g2eZ9s
	MessageID int64  `json:"message_id,omitempty"`
	Err       string `json:"error,omitempty"`
	// device group messages only, see https://goo.gl/kx9ENj
	FailedRegistrationIDs []string `json:"failed_registration_ids,omitempty"`
}

type result struct {
	// topic message id when GCM has successfully received the request and will
	// attempt to deliver to all subscribed devices
	MessageID string `json:"message_id,omitempty"`
	// canonical registration token for the client app that the message was
	// processed and sent to.
	RegistrationID string `json:"registration_id,omitempty"`
	Err            string `json:"error,omitempty"`
}

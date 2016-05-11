package gcm

// Result represents the status of a processed message.
//
// Some fields are specific to device group messages: Success, Failure, FailedRegistrationIDs.
type Result struct {
	MessageID               string `json:"message_id,omitempty"`
	CanonicalRegistrationID string `json:"canonical_registration_id,omitempty"`
	Error                   string `json:"error,omitempty"`
	// device group message only
	Success               int      `json:"success,omitempty"`
	Failure               int      `json:"failure,omitempty"`
	FailedRegistrationIDs []string `json:"failed_registration_ids,omitempty"`
}

// MulticastResult represents the response of a processed multicast message.
type MulticastResult struct {
	Success           int      `json:"success"`
	Failure           int      `json:"failure"`
	CanonicalIds      int      `json:"canonical_ids"`
	MulticastID       int64    `json:"multicast_id"`
	Results           []Result `json:"results,omitempty"`
	RetryMulticastIDs []int64  `json:"retry_multicast_ids,omitempty"`
}

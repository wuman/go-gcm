package gcm

type Result struct {
	MessageId               string `json:"message_id,omitempty"`
	CanonicalRegistrationId string `json:"canonical_registration_id,omitempty"`
	Error                   string `json:"error,omitempty"`
	// device group message only
	Success               int      `json:"success,omitempty"`
	Failure               int      `json:"failure,omitempty"`
	FailedRegistrationIds []string `json:"failed_registration_ids,omitempty"`
}

type MulticastResult struct {
	Success           int      `json:"success"`
	Failure           int      `json:"failure"`
	CanonicalIds      int      `json:"canonical_ids"`
	MulticastId       int64    `json:"multicast_id"`
	Results           []Result `json:"results,omitempty"`
	RetryMulticastIds []int64  `json:"retry_multicast_ids,omitempty"`
}

package gcm

import (
	"encoding/json"
	"fmt"
)

// Priority defines the priority of the message.
type Priority int

const (
	// PriorityNormal defines the "normal" value of priority.  On iOS, this
	// corresponds to APNs priority 5.
	PriorityNormal = iota + 1
	// PriorityHigh defines the "high" value of priority.  On iOS, this
	// corresponds to APNs priority 10.
	PriorityHigh
)

// Message specifies the downstream HTTP messages in JSON format.
// Refer to https://goo.gl/ot271K.
type Message struct {
	// Options
	CollapseKey           string   `json:"collapse_key,omitempty"`
	DelayWhileIdle        bool     `json:"delay_while_idle,omitempty"`
	TimeToLive            int      `json:"time_to_live,omitempty"`
	RestrictedPackageName string   `json:"restricted_package_name,omitempty"`
	DryRun                bool     `json:"dry_run,omitempty"`
	ContentAvailable      bool     `json:"content_available,omitempty"`
	Priority              Priority `json:"priority,omitempty"`
	// Payload
	Data         map[string]string `json:"data,omitempty"`
	Notification *Notification     `json:"notification,omitempty"`
}

type message struct {
	Message
	// Targets
	to              string
	registrationIds []string
}

func (m *message) UnmarshalJSON(data []byte) error {
	var aux struct {
		To              string   `json:"to,omitempty"`
		RegistrationIDs []string `json:"registration_ids,omitempty"`
		Message
	}
	if err := json.Unmarshal(data, &aux); err != nil {
		return err
	}
	m.to = aux.To
	m.registrationIds = aux.RegistrationIDs
	m.Message = aux.Message
	return nil
}

// UnmarshalJSON unmarshals Priority from json.
func (p *Priority) UnmarshalJSON(data []byte) error {
	var s string
	if err := json.Unmarshal(data, &s); err != nil {
		return fmt.Errorf("priority should be a string, got %v", data)
	}

	switch s {
	case "normal":
		*p = PriorityNormal
	case "high":
		*p = PriorityHigh
	default:
		return fmt.Errorf("priority should be either normal or high, got %s", s)
	}
	return nil
}

// MarshalJSON marshals Priority to json.
func (p Priority) MarshalJSON() ([]byte, error) {
	switch p {
	case PriorityNormal:
		return json.Marshal("normal")
	case PriorityHigh:
		return json.Marshal("high")
	default:
		return nil, fmt.Errorf("invalid priority value: %v", p)
	}
}

func (m message) MarshalJSON() ([]byte, error) {
	aux := struct {
		Message
		To              string   `json:"to,omitempty"`
		RegistrationIDs []string `json:"registration_ids,omitempty"`
	}{
		Message:         m.Message,
		To:              m.to,
		RegistrationIDs: m.registrationIds,
	}
	return json.Marshal(aux)
}

// Notification is the notification payload as defined at https://goo.gl/ChtnMw.
type Notification struct {
	Title        string   `json:"title,omitempty"` // required for Android
	Body         string   `json:"body,omitempty"`
	Sound        string   `json:"sound,omitempty"`
	ClickAction  string   `json:"click_action,omitempty"`
	BodyLocKey   string   `json:"body_loc_key,omitempty"`
	BodyLocArgs  []string `json:"body_loc_args,omitempty"`
	TitleLocKey  string   `json:"title_loc_key,omitempty"`
	TitleLocArgs []string `json:"title_loc_args,omitempty"`
	// Android only
	Icon  string `json:"icon,omitempty"`
	Tag   string `json:"tag,omitempty"`
	Color string `json:"color,omitempty"`
	// iOS only
	Badge string `json:"badge,omitempty"`
}

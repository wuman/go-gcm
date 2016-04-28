package gcm

import (
	"encoding/json"
	"fmt"
)

type Priority int

const (
	Priority_Normal = iota + 1
	Priority_High
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
	to              string   `json:"to,omitempty"`
	registrationIds []string `json:"registration_ids,omitempty"`
}

func (m *message) UnmarshalJSON(data []byte) error {
	var aux struct {
		To              string   `json:"to,omitempty"`
		RegistrationIds []string `json:"registration_ids,omitempty"`
		Message
	}
	if err := json.Unmarshal(data, &aux); err != nil {
		return err
	}
	m.to = aux.To
	m.registrationIds = aux.RegistrationIds
	m.Message = aux.Message
	return nil
}

func (p *Priority) UnmarshalJSON(data []byte) error {
	var s string
	if err := json.Unmarshal(data, &s); err != nil {
		return fmt.Errorf("priority should be a string, got %v", data)
	}

	switch s {
	case "normal":
		*p = Priority_Normal
	case "high":
		*p = Priority_High
	default:
		return fmt.Errorf("priority should be either normal or high, got %s", s)
	}
	return nil
}

func (p Priority) MarshalJSON() ([]byte, error) {
	switch p {
	case Priority_Normal:
		return json.Marshal("normal")
	case Priority_High:
		return json.Marshal("high")
	default:
		return nil, fmt.Errorf("invalid priority value: %v", p)
	}
}

func (m message) MarshalJSON() ([]byte, error) {
	aux := struct {
		Message
		To              string   `json:"to,omitempty"`
		RegistrationIds []string `json:"registration_ids,omitempty"`
	}{
		Message:         m.Message,
		To:              m.to,
		RegistrationIds: m.registrationIds,
	}
	return json.Marshal(aux)
}

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

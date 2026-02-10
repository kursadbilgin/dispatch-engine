package domain

import (
	"errors"
	"strings"
	"testing"
)

func TestParseStatusFromString(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		input   string
		want    Status
		wantErr bool
	}{
		{name: "valid uppercase", input: "SENT", want: StatusSent},
		{name: "valid lowercase with spaces", input: " queued ", want: StatusQueued},
		{name: "invalid", input: "unknown", wantErr: true},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := ParseStatusFromString(tt.input)
			if tt.wantErr {
				if !errors.Is(err, ErrValidation) {
					t.Fatalf("ParseStatusFromString() error = %v, want ErrValidation", err)
				}
				return
			}

			if err != nil {
				t.Fatalf("ParseStatusFromString() unexpected error = %v", err)
			}
			if got != tt.want {
				t.Fatalf("ParseStatusFromString() = %s, want %s", got, tt.want)
			}
		})
	}
}

func TestParseChannelFromString(t *testing.T) {
	t.Parallel()

	got, err := ParseChannelFromString(" sms ")
	if err != nil {
		t.Fatalf("ParseChannelFromString() unexpected error = %v", err)
	}
	if got != ChannelSMS {
		t.Fatalf("ParseChannelFromString() = %s, want %s", got, ChannelSMS)
	}

	_, err = ParseChannelFromString("fax")
	if !errors.Is(err, ErrValidation) {
		t.Fatalf("ParseChannelFromString() error = %v, want ErrValidation", err)
	}
}

func TestParsePriorityFromString(t *testing.T) {
	t.Parallel()

	got, err := ParsePriorityFromString(" high ")
	if err != nil {
		t.Fatalf("ParsePriorityFromString() unexpected error = %v", err)
	}
	if got != PriorityHigh {
		t.Fatalf("ParsePriorityFromString() = %s, want %s", got, PriorityHigh)
	}

	_, err = ParsePriorityFromString("urgent")
	if !errors.Is(err, ErrValidation) {
		t.Fatalf("ParsePriorityFromString() error = %v, want ErrValidation", err)
	}
}

func TestNotificationValidate(t *testing.T) {
	t.Parallel()

	base := Notification{
		Channel:   ChannelSMS,
		Priority:  PriorityNormal,
		Recipient: "+905551112233",
		Content:   "hello",
	}

	tests := []struct {
		name    string
		mutate  func(*Notification)
		wantErr bool
	}{
		{
			name: "valid notification",
			mutate: func(n *Notification) {
				// keep base
			},
		},
		{
			name: "missing recipient",
			mutate: func(n *Notification) {
				n.Recipient = ""
			},
			wantErr: true,
		},
		{
			name: "missing content",
			mutate: func(n *Notification) {
				n.Content = ""
			},
			wantErr: true,
		},
		{
			name: "invalid channel",
			mutate: func(n *Notification) {
				n.Channel = Channel("VOICE")
			},
			wantErr: true,
		},
		{
			name: "invalid priority",
			mutate: func(n *Notification) {
				n.Priority = Priority("URGENT")
			},
			wantErr: true,
		},
		{
			name: "sms content over limit",
			mutate: func(n *Notification) {
				n.Channel = ChannelSMS
				n.Content = strings.Repeat("a", MaxSMSContent+1)
			},
			wantErr: true,
		},
		{
			name: "push content over limit",
			mutate: func(n *Notification) {
				n.Channel = ChannelPush
				n.Content = strings.Repeat("a", MaxPushContent+1)
			},
			wantErr: true,
		},
		{
			name: "email content over limit",
			mutate: func(n *Notification) {
				n.Channel = ChannelEmail
				n.Content = strings.Repeat("a", MaxEmailContent+1)
			},
			wantErr: true,
		},
		{
			name: "rune-aware sms length accepted",
			mutate: func(n *Notification) {
				n.Channel = ChannelSMS
				n.Content = strings.Repeat("ğ", MaxSMSContent)
			},
		},
		{
			name: "rune-aware sms length overflow",
			mutate: func(n *Notification) {
				n.Channel = ChannelSMS
				n.Content = strings.Repeat("ğ", MaxSMSContent+1)
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			current := base
			tt.mutate(&current)

			err := current.Validate()
			if tt.wantErr {
				if !errors.Is(err, ErrValidation) {
					t.Fatalf("Validate() error = %v, want ErrValidation", err)
				}
				return
			}

			if err != nil {
				t.Fatalf("Validate() unexpected error = %v", err)
			}
		})
	}
}

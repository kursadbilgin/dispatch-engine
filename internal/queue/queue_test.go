package queue

import (
	"testing"

	"github.com/kursadbilgin/dispatch-engine/internal/domain"
)

func TestQueueNames(t *testing.T) {
	work := WorkQueueNames()
	if len(work) != 3 {
		t.Fatalf("WorkQueueNames len = %d, want 3", len(work))
	}

	expected := map[string]struct{}{
		"sms":   {},
		"email": {},
		"push":  {},
	}

	for _, name := range work {
		if _, ok := expected[name]; !ok {
			t.Fatalf("unexpected queue name: %s", name)
		}
	}

	dlq := DLQNames()
	if len(dlq) != 3 {
		t.Fatalf("DLQNames len = %d, want 3", len(dlq))
	}

	expectedDLQ := map[string]struct{}{
		"dlq.sms":   {},
		"dlq.email": {},
		"dlq.push":  {},
	}

	for _, name := range dlq {
		if _, ok := expectedDLQ[name]; !ok {
			t.Fatalf("unexpected dlq name: %s", name)
		}
	}
}

func TestQueueName(t *testing.T) {
	queueName := QueueName(domain.ChannelSMS)
	if queueName != "sms" {
		t.Fatalf("QueueName = %s, want sms", queueName)
	}

	dlqName := DLQName(domain.ChannelEmail)
	if dlqName != "dlq.email" {
		t.Fatalf("DLQName = %s, want dlq.email", dlqName)
	}
}

func TestPriorityValue(t *testing.T) {
	tests := []struct {
		name     string
		priority domain.Priority
		want     uint8
	}{
		{name: "high", priority: domain.PriorityHigh, want: 3},
		{name: "normal", priority: domain.PriorityNormal, want: 2},
		{name: "low", priority: domain.PriorityLow, want: 1},
		{name: "invalid", priority: domain.Priority("invalid"), want: 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := PriorityValue(tt.priority)
			if got != tt.want {
				t.Fatalf("PriorityValue(%q) = %d, want %d", tt.priority, got, tt.want)
			}
		})
	}
}

func TestNotificationMessageValidate(t *testing.T) {
	msg := NotificationMessage{
		NotificationID: "n1",
		Channel:        domain.ChannelSMS,
		Priority:       domain.PriorityNormal,
	}
	if err := msg.Validate(); err != nil {
		t.Fatalf("Validate() unexpected error: %v", err)
	}

	msg.NotificationID = ""
	if err := msg.Validate(); err == nil {
		t.Fatal("expected error for empty notification id")
	}

	msg.NotificationID = "n1"
	msg.Channel = domain.Channel("invalid")
	if err := msg.Validate(); err == nil {
		t.Fatal("expected error for invalid channel")
	}

	msg.Channel = domain.ChannelSMS
	msg.Priority = domain.Priority("invalid")
	if err := msg.Validate(); err == nil {
		t.Fatal("expected error for invalid priority")
	}
}

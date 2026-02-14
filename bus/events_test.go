package bus

import "testing"

func TestInboundMessageSessionKeyIncludesAccountDimension(t *testing.T) {
	cases := []struct {
		name string
		a    InboundMessage
		b    InboundMessage
	}{
		{
			name: "same channel/chat different accounts",
			a: InboundMessage{
				Channel:   "telegram",
				AccountID: "acc-a",
				ChatID:    "chat-1",
			},
			b: InboundMessage{
				Channel:   "telegram",
				AccountID: "acc-b",
				ChatID:    "chat-1",
			},
		},
		{
			name: "same channel/chat one default one explicit account",
			a: InboundMessage{
				Channel:   "qq",
				AccountID: "",
				ChatID:    "group-42",
			},
			b: InboundMessage{
				Channel:   "qq",
				AccountID: "default",
				ChatID:    "group-42",
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.a.SessionKey() == tc.b.SessionKey() {
				t.Fatalf("expected different session keys, got same key %q", tc.a.SessionKey())
			}
		})
	}
}

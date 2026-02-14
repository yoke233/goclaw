package session

import (
	"fmt"
	"time"
)

func buildSessionWithMessages(key string, count int) *Session {
	now := time.Now()
	msgs := make([]Message, 0, count)
	for i := 0; i < count; i++ {
		msgs = append(msgs, Message{
			Role:      "user",
			Content:   fmt.Sprintf("m%d", i),
			Timestamp: now.Add(time.Duration(i) * time.Minute),
			Metadata: map[string]interface{}{
				"index": i,
			},
		})
	}

	return &Session{
		Key:       key,
		Messages:  msgs,
		CreatedAt: now,
		UpdatedAt: now,
		Metadata:  map[string]interface{}{},
	}
}

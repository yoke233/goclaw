package agent

// extractTextContent extracts first text block from an agent message.
func extractTextContent(msg AgentMessage) string {
	for _, block := range msg.Content {
		if text, ok := block.(TextContent); ok {
			return text.Text
		}
	}
	return ""
}

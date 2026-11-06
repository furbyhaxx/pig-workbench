package api

// SessionInfo describes one live session; it never carries the endpoint token.
type SessionInfo struct {
	SessionID   string `json:"sessionId"`
	Name        string `json:"name,omitempty"`
	ProjectID   string `json:"projectId,omitempty"`
	ProjectSlug string `json:"projectSlug,omitempty"`
	Cwd         string `json:"cwd,omitempty"`
	State       string `json:"state"`
	Model       string `json:"model,omitempty"`
}

// SessionsListResult answers sessions.list with the discoverable live sessions other than the caller's own.
type SessionsListResult struct {
	Sessions []SessionInfo `json:"sessions"`
}

// SessionsSendParams is the request of sessions.send. Channel is "agent" or "ext:<owner>".
type SessionsSendParams struct {
	Target      string   `json:"target"`
	Channel     string   `json:"channel"`
	Subject     string   `json:"subject,omitempty"`
	Body        string   `json:"body"`
	Attachments []string `json:"attachments,omitempty"`
	Wake        string   `json:"wake,omitempty"`
}

// SessionsSendResult is the synchronous receipt of sessions.send.
type SessionsSendResult struct {
	Status         string `json:"status"`
	Reason         string `json:"reason,omitempty"`
	Delivery       string `json:"delivery,omitempty"`
	MessageID      string `json:"messageId,omitempty"`
	ConversationID string `json:"conversationId,omitempty"`
}

// SessionsMessageEvent is the data of a sessions.message event, delivered for ext: channels.
type SessionsMessageEvent struct {
	Owner     string `json:"owner"`
	FromID    string `json:"fromSessionId"`
	FromName  string `json:"fromName,omitempty"`
	MessageID string `json:"messageId"`
	Subject   string `json:"subject,omitempty"`
	Body      string `json:"body"`
}

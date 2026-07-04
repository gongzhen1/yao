package mail

import (
	"context"
	"fmt"

	"github.com/yaoapp/gou/process"
	"github.com/yaoapp/kun/exception"
	"github.com/yaoapp/kun/log"
	"github.com/yaoapp/yao/messenger"
	messengerTypes "github.com/yaoapp/yao/messenger/types"
)

// ProcessHandlers registers mail-related process handlers for script invocation.
// Usage in scripts:
//
//	Process("mail.send", channel, to, subject, body)
//	Process("mail.send", channel, to, subject, body, html)
//	Process("mail.send", channel, to, subject, body, html, attachments)
var ProcessHandlers = map[string]process.Handler{
	"send": processMailSend,
}

func init() {
	process.RegisterGroup("mail", ProcessHandlers)
}

// processMailSend sends an email via the messenger service.
//
// Arguments:
//
//	args[0] - channel (string): the messenger channel name (e.g. "email")
//	args[1] - to (string | []interface{}): recipient email address(es)
//	args[2] - subject (string): email subject
//	args[3] - body (string): plain text body
//	args[4] - html (string, optional): HTML body content
//	args[5] - attachments ([]interface{}, optional): list of attachment objects
//	          each with: filename (string), content_type (string), content (string/base64)
func processMailSend(proc *process.Process) interface{} {
	args := proc.Args
	if len(args) < 4 {
		exception.New("mail.send requires at least 4 arguments: channel, to, subject, body", 400).Throw()
	}

	channel, ok := args[0].(string)
	if !ok {
		exception.New("mail.send: channel must be a string", 400).Throw()
	}

	to, err := parseRecipients(args[1])
	if err != nil {
		exception.New("mail.send: %v", 400, err).Throw()
	}

	subject, ok := args[2].(string)
	if !ok {
		exception.New("mail.send: subject must be a string", 400).Throw()
	}

	body, ok := args[3].(string)
	if !ok {
		exception.New("mail.send: body must be a string", 400).Throw()
	}

	html := ""
	if len(args) > 4 {
		if h, ok := args[4].(string); ok {
			html = h
		}
	}

	var attachments []messengerTypes.Attachment
	if len(args) > 5 {
		attachments = parseAttachments(args[5])
	}

	msg := &messengerTypes.Message{
		To:          to,
		Subject:     subject,
		Body:        body,
		HTML:        html,
		Type:        messengerTypes.MessageTypeEmail,
		Attachments: attachments,
	}

	svc := messenger.Instance
	if svc == nil {
		exception.New("mail.send: messenger service not available", 500).Throw()
	}

	ctx := proc.Context
	if ctx == nil {
		ctx = context.Background()
	}

	if err := svc.Send(ctx, channel, msg); err != nil {
		exception.New("mail.send: %v", 500, err).Throw()
	}

	log.Info("[Mail] send success via channel=%s to=%v subject=%q", channel, to, subject)
	return map[string]interface{}{
		"success":    true,
		"channel":    channel,
		"recipients": to,
		"subject":    subject,
	}
}

// parseRecipients converts a string or []interface{} to []string.
func parseRecipients(arg interface{}) ([]string, error) {
	switch v := arg.(type) {
	case string:
		if v == "" {
			return nil, fmt.Errorf("to recipient must not be empty")
		}
		return []string{v}, nil
	case []interface{}:
		if len(v) == 0 {
			return nil, fmt.Errorf("to recipients list must not be empty")
		}
		result := make([]string, 0, len(v))
		for i, item := range v {
			s, ok := item.(string)
			if !ok {
				return nil, fmt.Errorf("to[%d] must be a string", i)
			}
			if s != "" {
				result = append(result, s)
			}
		}
		if len(result) == 0 {
			return nil, fmt.Errorf("to recipients list contains no valid addresses")
		}
		return result, nil
	case []string:
		if len(v) == 0 {
			return nil, fmt.Errorf("to recipients list must not be empty")
		}
		return v, nil
	default:
		return nil, fmt.Errorf("to must be a string or list of strings, got %T", arg)
	}
}

// parseAttachments converts script-friendly attachment data into messengerTypes.Attachment.
func parseAttachments(arg interface{}) []messengerTypes.Attachment {
	items, ok := arg.([]interface{})
	if !ok {
		log.Warn("[Mail] attachments must be an array, got %T", arg)
		return nil
	}

	result := make([]messengerTypes.Attachment, 0, len(items))
	for i, item := range items {
		m, ok := item.(map[string]interface{})
		if !ok {
			log.Warn("[Mail] attachment[%d] must be an object, skipping", i)
			continue
		}

		att := messengerTypes.Attachment{}

		if filename, ok := m["filename"].(string); ok {
			att.Filename = filename
		}
		if contentType, ok := m["content_type"].(string); ok {
			att.ContentType = contentType
		}
		if inline, ok := m["inline"].(bool); ok {
			att.Inline = inline
		}
		if cid, ok := m["cid"].(string); ok {
			att.CID = cid
		}

		// Content can be a string (base64 or plain) or []byte
		switch c := m["content"].(type) {
		case string:
			att.Content = []byte(c)
		case []byte:
			att.Content = c
		default:
			log.Warn("[Mail] attachment[%d] content must be a string or bytes, skipping", i)
			continue
		}

		result = append(result, att)
	}
	return result
}

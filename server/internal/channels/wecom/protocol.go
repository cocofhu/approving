package wecom

import (
	"encoding/json"

	"github.com/google/uuid"
)

const (
	wsURL = "wss://openws.work.weixin.qq.com"

	cmdSubscribe    = "aibot_subscribe"
	cmdCallback     = "aibot_msg_callback"
	cmdRespond      = "aibot_respond_msg"
	cmdSend         = "aibot_send_msg"
	cmdPing         = "ping"
	cmdDisconnected = "disconnected_event"

	// Inbound callback chattype (string).
	chatSingle = "single"
	chatGroup  = "group"

	// Official aibot_send_msg chat_type (uint32): 1 = C2C, 2 = group.
	sendChatTypeSingle uint32 = 1
	sendChatTypeGroup  uint32 = 2

	msgText  = "text"
	msgImage = "image"
	msgMixed = "mixed"
)

type frame struct {
	Cmd     string          `json:"cmd"`
	Headers frameHeaders    `json:"headers"`
	Body    json.RawMessage `json:"body,omitempty"`
	ErrCode int             `json:"errcode,omitempty"`
	ErrMsg  string          `json:"errmsg,omitempty"`
}

type frameHeaders struct {
	ReqID string `json:"req_id"`
}

type subscribeBody struct {
	BotID  string `json:"bot_id"`
	Secret string `json:"secret"`
}

type resultBody struct {
	ErrCode int    `json:"errcode"`
	ErrMsg  string `json:"errmsg"`
}

type callbackBody struct {
	MsgID    string         `json:"msgid"`
	AIBotID  string         `json:"aibotid"`
	ChatID   string         `json:"chatid"`
	ChatType string         `json:"chattype"`
	From     callbackFrom   `json:"from"`
	MsgType  string         `json:"msgtype"`
	Text     *callbackText  `json:"text,omitempty"`
	Image    *callbackImage `json:"image,omitempty"`
	Mixed    *callbackMixed `json:"mixed,omitempty"`
}

// Official mixed payload is a list of text/image items, not a sibling image field.
type callbackMixed struct {
	Item    []mixedItem `json:"item"`
	MsgItem []mixedItem `json:"msg_item"`
}

type mixedItem struct {
	Type  string         `json:"type"`
	Text  *callbackText  `json:"text,omitempty"`
	Image *callbackImage `json:"image,omitempty"`
}

func (m *callbackMixed) items() []mixedItem {
	if m == nil {
		return nil
	}
	if len(m.Item) > 0 {
		return m.Item
	}
	return m.MsgItem
}

type callbackFrom struct {
	UserID string `json:"userid"`
}

type callbackText struct {
	Content string `json:"content"`
}

type callbackImage struct {
	URL    string `json:"url"`
	AESKey string `json:"aeskey"`
}

type markdownBody struct {
	Content string `json:"content"`
}

type respondBody struct {
	MsgID    string       `json:"msgid,omitempty"`
	MsgType  string       `json:"msgtype"`
	Markdown markdownBody `json:"markdown"`
	Finish   bool         `json:"finish"`
}

type sendBody struct {
	ChatID   string       `json:"chatid"`
	ChatType uint32       `json:"chat_type"`
	MsgType  string       `json:"msgtype"`
	Markdown markdownBody `json:"markdown"`
}

func newReqID() string {
	return uuid.NewString()
}

func encodeFrame(cmd, reqID string, body any) ([]byte, error) {
	f := frame{
		Cmd:     cmd,
		Headers: frameHeaders{ReqID: reqID},
	}
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		f.Body = raw
	}
	return json.Marshal(f)
}

func decodeResult(f frame) (int, string) {
	code, msg := f.ErrCode, f.ErrMsg
	if len(f.Body) > 0 {
		var rb resultBody
		if json.Unmarshal(f.Body, &rb) == nil {
			if rb.ErrCode != 0 {
				code = rb.ErrCode
			}
			if rb.ErrMsg != "" {
				msg = rb.ErrMsg
			}
		}
	}
	return code, msg
}

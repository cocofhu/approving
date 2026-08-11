package wecom

import (
	"bytes"
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"

	"github.com/cocofhu/approving/internal/channels"
	"github.com/cocofhu/approving/internal/models"
)

func TestTruncateMarkdown(t *testing.T) {
	if got := TruncateMarkdown("短"); got != "短" {
		t.Fatalf("short = %q", got)
	}
	long := strings.Repeat("测", MarkdownMaxBytes)
	got := TruncateMarkdown(long)
	if !strings.HasSuffix(got, truncateSuffix) {
		t.Fatalf("missing suffix: %q", got[len(got)-12:])
	}
	if len(got) > MarkdownMaxBytes {
		t.Fatalf("len=%d want <= %d", len(got), MarkdownMaxBytes)
	}
	if !strings.HasPrefix(got, "测") {
		t.Fatal("should keep prefix")
	}
}

func TestClassifyWeComErrorRateLimit(t *testing.T) {
	err := classifyWeComError(45009, "api freq out of limit")
	if !IsRateLimited(err) {
		t.Fatalf("45009 should be rate limit: %v", err)
	}
	err = classifyWeComError(0, "访问频率过高")
	if !IsRateLimited(err) {
		t.Fatalf("频率文案 should be rate limit: %v", err)
	}
	err = classifyWeComError(40014, "invalid secret")
	if IsRateLimited(err) || IsUnspoken(err) {
		t.Fatalf("generic should not classify: %v", err)
	}
}

func TestDecryptImageAES256CBC(t *testing.T) {
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		t.Fatal(err)
	}
	plain := []byte("wecom-image-plain-bytes!!")
	block, err := aes.NewCipher(key)
	if err != nil {
		t.Fatal(err)
	}
	pad := aes.BlockSize - len(plain)%aes.BlockSize
	padded := append(plain, bytes.Repeat([]byte{byte(pad)}, pad)...)
	ct := make([]byte, len(padded))
	cipher.NewCBCEncrypter(block, key[:16]).CryptBlocks(ct, padded)

	got, err := DecryptImageAES256CBC(ct, hex.EncodeToString(key))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, plain) {
		t.Fatalf("got %q want %q", got, plain)
	}
}

func TestInboundMapsSingleAndGroup(t *testing.T) {
	a, err := New(channels.AdapterConfig{AppID: "bot", AppSecret: "sec"})
	if err != nil {
		t.Fatal(err)
	}
	ad := a.(*Adapter)
	var got []channels.InboundMessage
	var mu sync.Mutex
	on := func(_ context.Context, in channels.InboundMessage) {
		mu.Lock()
		got = append(got, in)
		mu.Unlock()
	}

	singleBody, _ := json.Marshal(callbackBody{
		MsgID: "m1", ChatType: chatSingle, From: callbackFrom{UserID: "zhangsan"},
		MsgType: msgText, Text: &callbackText{Content: "hello"},
	})
	ad.ApplyCallbackForTest(context.Background(), frame{
		Cmd: cmdCallback, Headers: frameHeaders{ReqID: "req-1"}, Body: singleBody,
	}, on)

	groupBody, _ := json.Marshal(callbackBody{
		MsgID: "m2", ChatType: chatGroup, ChatID: "wr_ops",
		From: callbackFrom{UserID: "lisi"}, MsgType: msgText, Text: &callbackText{Content: "群消息"},
	})
	ad.ApplyCallbackForTest(context.Background(), frame{
		Cmd: cmdCallback, Headers: frameHeaders{ReqID: "req-2"}, Body: groupBody,
	}, on)

	// duplicate msgid ignored
	ad.ApplyCallbackForTest(context.Background(), frame{
		Cmd: cmdCallback, Headers: frameHeaders{ReqID: "req-1b"}, Body: singleBody,
	}, on)

	// group image ignored
	imgBody, _ := json.Marshal(callbackBody{
		MsgID: "m3", ChatType: chatGroup, ChatID: "wr_ops",
		From: callbackFrom{UserID: "lisi"}, MsgType: msgImage,
		Image: &callbackImage{URL: "https://x/a", AESKey: "k"},
	})
	ad.ApplyCallbackForTest(context.Background(), frame{
		Cmd: cmdCallback, Headers: frameHeaders{ReqID: "req-3"}, Body: imgBody,
	}, on)

	mu.Lock()
	defer mu.Unlock()
	if len(got) != 2 {
		t.Fatalf("inbound count=%d want 2: %+v", len(got), got)
	}
	if got[0].Scene != channels.SceneC2C || got[0].ConversationID != "zhangsan" || got[0].Text != "hello" {
		t.Fatalf("single = %+v", got[0])
	}
	if ad.lookupReqID("m1") != "req-1" {
		t.Fatalf("req_id cache missing")
	}
	if got[1].Scene != channels.SceneGroup || got[1].ConversationID != "wr_ops" {
		t.Fatalf("group = %+v", got[1])
	}
}

func TestSendUnspokenAndOffline(t *testing.T) {
	a, err := New(channels.AdapterConfig{AppID: "bot", AppSecret: "sec"})
	if err != nil {
		t.Fatal(err)
	}
	ad := a.(*Adapter)
	ad.SetSpokenCheckerForTest(func(scene channels.Scene, conversationID string) bool {
		return false
	})
	err = ad.Send(context.Background(), channels.OutboundMessage{
		Scene: channels.SceneC2C, ConversationID: "silent", Text: "hi",
	})
	if !IsUnspoken(err) {
		t.Fatalf("want unspoken, got %v", err)
	}

	ad.SetSpokenCheckerForTest(func(scene channels.Scene, conversationID string) bool { return true })
	err = ad.Send(context.Background(), channels.OutboundMessage{
		Scene: channels.SceneC2C, ConversationID: "u1", Text: "hi",
	})
	if !errors.Is(err, ErrOffline) {
		t.Fatalf("want offline, got %v", err)
	}
}

func TestTypeConstant(t *testing.T) {
	a, _ := New(channels.AdapterConfig{AppID: "b", AppSecret: "s"})
	if a.Type() != models.ChannelTypeWeCom {
		t.Fatalf("type=%s", a.Type())
	}
	if a.(*Adapter).Online() {
		t.Fatal("should be offline before subscribe")
	}
}

func TestDownloadDecryptImage(t *testing.T) {
	key := bytes.Repeat([]byte{0x11}, 32)
	plain := []byte("png-bytes-here")
	block, _ := aes.NewCipher(key)
	pad := aes.BlockSize - len(plain)%aes.BlockSize
	padded := append(plain, bytes.Repeat([]byte{byte(pad)}, pad)...)
	ct := make([]byte, len(padded))
	cipher.NewCBCEncrypter(block, key[:16]).CryptBlocks(ct, padded)

	a, _ := New(channels.AdapterConfig{AppID: "b", AppSecret: "s"})
	ad := a.(*Adapter)
	ad.httpDo = func(req *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: 200,
			Body:       io.NopCloser(bytes.NewReader(ct)),
		}, nil
	}
	img, hint := ad.downloadDecryptImage(context.Background(), &callbackImage{
		URL: "https://example.com/img", AESKey: hex.EncodeToString(key),
	})
	if hint != "" || img == nil {
		t.Fatalf("hint=%q img=%v", hint, img)
	}
	if !bytes.Equal(img.Data, plain) {
		t.Fatalf("data=%q", img.Data)
	}
}

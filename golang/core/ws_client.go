package core

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log"
	"math/rand"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/vmihailenco/msgpack/v5"

	"xianyu-cli/models"
	"xianyu-cli/utils"
)

const (
	heartbeatInterval = 15 * time.Second
	reconnectDelay    = 5 * time.Second
	maxReconnects     = 3
)

func generateMID() string {
	randomPart := int(1000 * rand.Float64())
	timestamp := time.Now().UnixMilli()
	return fmt.Sprintf("%d%d 0", randomPart, timestamp)
}

// GoofishWebSocket is the WebSocket client for Goofish real-time messaging.
// Uses a single reader goroutine with a channel to avoid gorilla/websocket's
// "repeated read on failed connection" panic from SetReadDeadline timeouts.
type GoofishWebSocket struct {
	cookies     map[string]string
	accessToken string
	userID      string
	deviceID    string
	ws          *websocket.Conn
	running     bool
	writeMu     sync.Mutex
	incomingCh  chan []byte // single reader goroutine pushes all messages here
	pendingBuf  [][]byte   // messages consumed by CreateChat/SendMessage but not matched — re-queued for Watch
	pendingMu   sync.Mutex
	stopCh      chan struct{}
}

// NewGoofishWebSocket creates a new WebSocket client.
func NewGoofishWebSocket(cookies map[string]string, accessToken, userID, deviceID string) *GoofishWebSocket {
	if deviceID == "" {
		deviceID = fmt.Sprintf("%x", rand.Int63())[:16]
	}
	return &GoofishWebSocket{
		cookies:     cookies,
		accessToken: accessToken,
		userID:      userID,
		deviceID:    deviceID,
	}
}

// writeJSON sends a JSON message, protected by mutex.
func (ws *GoofishWebSocket) writeJSON(v interface{}) error {
	data, err := json.Marshal(v)
	if err != nil {
		return err
	}
	ws.writeMu.Lock()
	defer ws.writeMu.Unlock()
	return ws.ws.WriteMessage(websocket.TextMessage, data)
}

// readNext reads the next message, draining the pending buffer first.
// Returns nil, nil on timeout (not an error — connection stays alive).
func (ws *GoofishWebSocket) readNext(timeout time.Duration) ([]byte, error) {
	// Drain pending buffer first (messages buffered during CreateChat/SendMessage)
	ws.pendingMu.Lock()
	if len(ws.pendingBuf) > 0 {
		msg := ws.pendingBuf[0]
		ws.pendingBuf = ws.pendingBuf[1:]
		ws.pendingMu.Unlock()
		return msg, nil
	}
	ws.pendingMu.Unlock()

	select {
	case msg, ok := <-ws.incomingCh:
		if !ok {
			return nil, fmt.Errorf("connection closed")
		}
		return msg, nil
	case <-time.After(timeout):
		return nil, nil // timeout, not a connection error
	}
}

// bufferMsg saves an unconsumed message for later processing by Watch/WatchFiltered.
func (ws *GoofishWebSocket) bufferMsg(msg []byte) {
	ws.pendingMu.Lock()
	ws.pendingBuf = append(ws.pendingBuf, msg)
	ws.pendingMu.Unlock()
}

// readerLoop is the ONLY goroutine that calls ws.ReadMessage().
// All other code reads from incomingCh via readNext().
func (ws *GoofishWebSocket) readerLoop() {
	defer close(ws.incomingCh)
	for {
		_, msg, err := ws.ws.ReadMessage()
		if err != nil {
			return
		}
		select {
		case ws.incomingCh <- msg:
		case <-ws.stopCh:
			return
		}
	}
}

// Connect establishes the WebSocket connection, registers, syncs, and starts heartbeat + reader.
func (ws *GoofishWebSocket) Connect(syncFromTS *int64) error {
	headers := make(map[string][]string)
	headers["Cookie"] = []string{utils.BuildCookieHeader(ws.cookies)}
	headers["User-Agent"] = []string{"Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/133.0.0.0 Safari/537.36"}
	headers["Origin"] = []string{utils.GoofishOrigin}

	dialer := websocket.Dialer{HandshakeTimeout: 30 * time.Second}
	conn, _, err := dialer.Dial(utils.WSSURL, headers)
	if err != nil {
		return fmt.Errorf("websocket connect failed: %w", err)
	}
	ws.ws = conn
	ws.incomingCh = make(chan []byte, 64)
	ws.stopCh = make(chan struct{})
	ws.running = true

	// Start reader goroutine BEFORE register so we can receive the response
	go ws.readerLoop()

	if err := ws.register(); err != nil {
		return err
	}
	if err := ws.syncAck(syncFromTS); err != nil {
		return err
	}

	go ws.heartbeatLoop()

	log.Println("WebSocket connected, registered, synced, and heartbeat started")
	return nil
}

func (ws *GoofishWebSocket) register() error {
	regMsg := map[string]interface{}{
		"lwp": "/reg",
		"headers": map[string]interface{}{
			"cache-header": "app-key token ua wv",
			"app-key":      utils.MsgAppKey,
			"token":        ws.accessToken,
			"ua":           "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36",
			"dt":           "j",
			"wv":           "im:3,au:3,sy:6",
			"sync":         "0,0;0;0;",
			"did":          ws.deviceID,
			"mid":          generateMID(),
		},
	}
	if err := ws.writeJSON(regMsg); err != nil {
		return err
	}

	// Wait for registration response — skip sync pushes
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		remaining := time.Until(deadline)
		raw, err := ws.readNext(remaining)
		if err != nil || raw == nil {
			break
		}
		var resp map[string]interface{}
		if json.Unmarshal(raw, &resp) != nil {
			continue
		}
		lwp, _ := resp["lwp"].(string)
		if lwp == "/s/sync" || lwp == "/s/para" || lwp == "/s/vulcan" || lwp == "/!" {
			continue
		}
		// Check code at root level (DingTalk puts code at root, not in headers)
		code := respCode(resp)
		if code == "200" {
			log.Printf("Registration successful")
			return nil
		}
		if code != "" {
			log.Printf("Registration response code: %s", code)
		}
	}
	log.Printf("No registration confirmation, proceeding anyway")
	return nil
}

func (ws *GoofishWebSocket) syncAck(syncFromTS *int64) error {
	nowMS := time.Now().UnixMilli()
	ptsMS := nowMS
	if syncFromTS != nil {
		ptsMS = *syncFromTS
	}
	ackMsg := map[string]interface{}{
		"lwp":     "/r/SyncStatus/ackDiff",
		"headers": map[string]interface{}{"mid": generateMID()},
		"body": []map[string]interface{}{{
			"pipeline": "sync", "tooLong2Tag": "PNM,1", "channel": "sync",
			"topic": "sync", "highPts": 0, "pts": ptsMS * 1000, "seq": 0, "timestamp": nowMS,
		}},
	}
	return ws.writeJSON(ackMsg)
}

// CreateChat creates a conversation with a user about a specific item.
// It sends the create request and waits for the server to return a CID.
// If no CID is returned, it constructs one from the user IDs.
func (ws *GoofishWebSocket) CreateChat(toUserID, itemID string) (string, error) {
	toUID := toUserID
	if !strings.HasSuffix(toUID, "@goofish") {
		toUID += "@goofish"
	}
	myUID := ws.userID
	if myUID != "" && !strings.HasSuffix(myUID, "@goofish") {
		myUID += "@goofish"
	}

	mid := generateMID()
	msg := map[string]interface{}{
		"lwp":     "/r/SingleChatConversation/create",
		"headers": map[string]interface{}{"mid": mid},
		"body": []map[string]interface{}{{
			"pairFirst": toUID, "pairSecond": myUID, "bizType": "1",
			"extension": map[string]interface{}{"itemId": itemID},
			"ctx":       map[string]interface{}{"appVersion": "1.0", "platform": "web"},
		}},
	}
	ws.writeJSON(msg)

	// Wait for CID response (20s timeout — server can take 10-15s).
	// DO NOT buffer any messages here — buffering causes infinite loops
	// since readNext drains pendingBuf first. Drop everything except CID responses.
	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		remaining := time.Until(deadline)
		raw, err := ws.readNext(remaining)
		if err != nil || raw == nil {
			break
		}

		var respData map[string]interface{}
		if json.Unmarshal(raw, &respData) != nil {
			continue // unparseable — drop
		}

		cid := extractCID(respData)
		if cid != "" {
			log.Printf("[CreateChat] got cid=%s from server", cid)
			return cid, nil
		}
		// All other messages (sync, ack, heartbeat) — drop, don't buffer
	}

	log.Printf("[CreateChat] no CID within timeout for user=%s item=%s", toUserID, itemID)
	return "", fmt.Errorf("no cid returned for user %s item %s", toUserID, itemID)
}

// SendMessage sends a text message to a user.
func (ws *GoofishWebSocket) SendMessage(conversationID, toUserID, text, itemID string) error {
	if conversationID == "" && itemID != "" {
		cid, err := ws.CreateChat(toUserID, itemID)
		if err != nil {
			return err
		}
		conversationID = cid
	}
	if conversationID == "" {
		return fmt.Errorf("cannot send message: no conversation_id and no item_id")
	}

	textContent := map[string]interface{}{
		"contentType": 1,
		"text":        map[string]interface{}{"text": text},
	}
	textJSON, _ := json.Marshal(textContent)
	textB64 := base64.StdEncoding.EncodeToString(textJSON)

	cid := conversationID
	if !strings.HasSuffix(cid, "@goofish") {
		cid += "@goofish"
	}
	toUID := toUserID
	if !strings.HasSuffix(toUID, "@goofish") {
		toUID += "@goofish"
	}
	myUID := ws.userID
	if myUID != "" && !strings.HasSuffix(myUID, "@goofish") {
		myUID += "@goofish"
	}

	msgUUID := fmt.Sprintf("-%d1", time.Now().UnixMilli())
	receivers := []string{toUID}
	if myUID != "" {
		receivers = append(receivers, myUID)
	}

	msg := map[string]interface{}{
		"lwp":     "/r/MessageSend/sendByReceiverScope",
		"headers": map[string]interface{}{"mid": generateMID()},
		"body": []interface{}{
			map[string]interface{}{
				"uuid": msgUUID, "cid": cid, "conversationType": 1,
				"content": map[string]interface{}{
					"contentType": 101,
					"custom":      map[string]interface{}{"type": 1, "data": textB64},
				},
				"redPointPolicy": 0, "extension": map[string]interface{}{"extJson": "{}"},
				"ctx": map[string]interface{}{"appVersion": "1.0", "platform": "web"},
				"mtags": map[string]interface{}{}, "msgReadStatusSetting": 1,
			},
			map[string]interface{}{"actualReceivers": receivers},
		},
	}
	ws.writeJSON(msg)

	// Wait for server confirmation
	deadline := time.Now().Add(8 * time.Second)
	for time.Now().Before(deadline) {
		remaining := time.Until(deadline)
		raw, err := ws.readNext(remaining)
		if err != nil || raw == nil {
			break
		}

		var respData map[string]interface{}
		if json.Unmarshal(raw, &respData) != nil {
			ws.bufferMsg(raw)
			continue
		}

		lwp, _ := respData["lwp"].(string)
		if lwp != "" && (strings.Contains(strings.ToLower(lwp), "sync") || lwp == "/s/vulcan" || lwp == "/s/para") {
			// Drop sync messages — DO NOT buffer (causes infinite loop in readNext)
			continue
		}

		code := respCode(respData)
		if code == "200" {
			return nil
		}
		if code != "" {
			body, _ := respData["body"].(map[string]interface{})
			reason := ""
			if body != nil {
				reason = fmt.Sprint(body["reason"])
			}
			return fmt.Errorf("server rejected message (code=%s): %s", code, reason)
		}
		ws.bufferMsg(raw)
	}

	log.Printf("No server confirmation for message to %s, assuming sent", toUserID)
	return nil
}

// ListConversations returns the conversation list.
func (ws *GoofishWebSocket) ListConversations() []map[string]interface{} {
	mid := generateMID()
	msg := map[string]interface{}{
		"lwp":     "/r/Conversation/listNewestPagination",
		"headers": map[string]interface{}{"mid": mid},
		"body":    []map[string]interface{}{{"pageSize": 50}},
	}
	ws.writeJSON(msg)

	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		remaining := time.Until(deadline)
		raw, err := ws.readNext(remaining)
		if err != nil || raw == nil {
			break
		}
		var respData map[string]interface{}
		if json.Unmarshal(raw, &respData) != nil {
			continue
		}
		lwp, _ := respData["lwp"].(string)
		if lwp == "/s/sync" || lwp == "/s/para" || lwp == "/s/vulcan" || lwp == "/!" {
			continue
		}
		log.Printf("[ListConv] response lwp=%s code=%s", lwp, respCode(respData))
		body, _ := respData["body"].(map[string]interface{})
		convs, _ := body["conversations"].([]interface{})
		if len(convs) > 0 {
			var result []map[string]interface{}
			for _, c := range convs {
				if m, ok := c.(map[string]interface{}); ok {
					result = append(result, m)
				}
			}
			return result
		}
	}
	return nil
}

// Watch watches for incoming messages in real-time.
func (ws *GoofishWebSocket) Watch(onMessage func(map[string]interface{}), timeout time.Duration) {
	start := time.Now()
	for ws.running {
		elapsed := time.Since(start)
		if elapsed >= timeout {
			break
		}
		remaining := timeout - elapsed
		if remaining > 30*time.Second {
			remaining = 30 * time.Second
		}

		raw, err := ws.readNext(remaining)
		if err != nil {
			break
		}
		if raw == nil {
			continue // timeout, keep going
		}

		for _, parsed := range ws.parsePushMessages(raw) {
			if onMessage != nil {
				onMessage(parsed)
			}
		}
	}
}

// WatchFiltered watches for messages from specific senders with auto-reconnect.
func (ws *GoofishWebSocket) WatchFiltered(targetSenderIDs map[string]bool, replies *[]map[string]interface{}, repliedIDs map[string]bool, timeout time.Duration) {
	startTime := time.Now()
	reconnectCount := 0
	lastSyncTS := time.Now().UnixMilli()

	for {
		err := ws.watchFilteredInner(targetSenderIDs, replies, repliedIDs, timeout, startTime, &lastSyncTS)
		if err == nil {
			return
		}

		elapsed := time.Since(startTime)
		reconnectCount++
		log.Printf("WebSocket disconnected after %.0fs (attempt %d/%d)", elapsed.Seconds(), reconnectCount, maxReconnects)

		if reconnectCount > maxReconnects || elapsed >= timeout {
			return
		}

		fmt.Fprintf(os.Stderr, "%s\n", utils.Dim.Sprintf("  WebSocket 断开，正在重连 (%d/%d)...", reconnectCount, maxReconnects))
		if err := ws.reconnect(&lastSyncTS); err != nil {
			fmt.Fprintf(os.Stderr, "%s\n", utils.Yellow.Sprintf("  重连失败: %v", err))
			return
		}
		fmt.Fprintln(os.Stderr, utils.Dim.Sprint("  重连成功，继续监听"))
	}
}

const allRepliedGracePeriod = 30 * time.Second // extra wait after all sellers replied

func (ws *GoofishWebSocket) watchFilteredInner(targetSenderIDs map[string]bool, replies *[]map[string]interface{}, repliedIDs map[string]bool, timeout time.Duration, startTime time.Time, lastSyncTS *int64) error {
	var allRepliedAt time.Time // zero means not all replied yet

	for ws.running {
		elapsed := time.Since(startTime)
		if elapsed >= timeout {
			return nil
		}
		remaining := timeout - elapsed
		if remaining > 30*time.Second {
			remaining = 30 * time.Second
		}

		raw, err := ws.readNext(remaining)
		if err != nil {
			return err // connection closed, trigger reconnect
		}
		if raw == nil {
			continue // timeout, keep going
		}

		*lastSyncTS = time.Now().UnixMilli()

		parsed := ws.parsePushMessages(raw)
		if len(parsed) > 0 {
			log.Printf("[WatchFiltered] Got %d parsed messages from frame", len(parsed))
		}

		for _, p := range parsed {
			senderID := fmt.Sprint(p["senderId"])
			if senderID == "" || senderID == "<nil>" {
				senderID = fmt.Sprint(p["senderUid"])
			}
			log.Printf("[WatchFiltered] Message from sender=%s, content=%v, isTarget=%v, isMe=%v",
				senderID, truncStr(fmt.Sprint(p["content"]), 50),
				targetSenderIDs[senderID], senderID == ws.userID)

			// Skip non-target senders and our own echo
			if !targetSenderIDs[senderID] || senderID == ws.userID {
				continue
			}

			content := fmt.Sprint(p["content"])
			if s, ok := p["content"].(string); ok {
				content = models.ParseMessageContent(s)
			}

			// Mark sender as having replied (for early-exit check),
			// but do NOT skip subsequent messages from the same sender
			repliedIDs[senderID] = true
			senderName := fmt.Sprint(p["senderNick"])
			if senderName == "" || senderName == "<nil>" {
				senderName = fmt.Sprint(p["senderName"])
			}

			*replies = append(*replies, map[string]interface{}{
				"seller_id": senderID, "seller_name": senderName,
				"content": content, "time": p["gmtCreate"],
			})
			log.Printf("Collected reply from %s (msg #%d, %d/%d sellers replied)",
				senderID, len(*replies), len(repliedIDs), len(targetSenderIDs))
		}

		// All targets replied — start grace period to collect follow-up messages
		allReplied := true
		for id := range targetSenderIDs {
			if !repliedIDs[id] {
				allReplied = false
				break
			}
		}
		if allReplied && allRepliedAt.IsZero() {
			allRepliedAt = time.Now()
			log.Printf("All %d sellers replied, waiting %v grace period for follow-up messages",
				len(targetSenderIDs), allRepliedGracePeriod)
		}
		if !allRepliedAt.IsZero() && time.Since(allRepliedAt) >= allRepliedGracePeriod {
			log.Printf("Grace period ended, collected %d total messages", len(*replies))
			return nil
		}
	}
	return nil
}

func (ws *GoofishWebSocket) reconnect(syncFromTS *int64) error {
	ws.running = false
	select {
	case ws.stopCh <- struct{}{}:
	default:
	}
	if ws.ws != nil {
		ws.ws.Close()
		ws.ws = nil
	}
	return ws.Connect(syncFromTS)
}

func (ws *GoofishWebSocket) heartbeatLoop() {
	ticker := time.NewTicker(heartbeatInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ws.stopCh:
			return
		case <-ticker.C:
			if !ws.running {
				return
			}
			hb := map[string]interface{}{
				"lwp":     "/!",
				"headers": map[string]interface{}{"mid": generateMID()},
			}
			if err := ws.writeJSON(hb); err != nil {
				log.Printf("Heartbeat failed: %v", err)
				return
			}
		}
	}
}

// ── Message parsing ──

func (ws *GoofishWebSocket) decodeSPPItem(item map[string]interface{}) map[string]interface{} {
	rawData, _ := item["data"].(string)
	if rawData == "" {
		return nil
	}
	biz, _ := item["bizType"].(float64)
	obj, _ := item["objectType"].(float64)

	// biz=40: chat-related messages (obj=40000 direct msg, obj=40006 session arouse, etc.)
	if int(biz) == 40 {
		// Try multiple base64 variants: standard, URL-safe, raw (no padding)
		var rawBytes []byte
		var b64err error
		for _, enc := range []*base64.Encoding{
			base64.StdEncoding,
			base64.RawStdEncoding,
			base64.URLEncoding,
			base64.RawURLEncoding,
		} {
			rawBytes, b64err = enc.DecodeString(rawData)
			if b64err == nil {
				break
			}
		}
		if b64err != nil {
			log.Printf("[decodeSPP] all base64 variants failed for biz=40 obj=%d: %v", int(obj), b64err)
			return nil
		}

		// Log first bytes for diagnosis
		header := rawBytes
		if len(header) > 16 {
			header = header[:16]
		}
		log.Printf("[decodeSPP] biz=40 obj=%d, decoded %d bytes, first16=%x", int(obj), len(rawBytes), header)

		// Primary: custom MessagePack decoder (handles integer keys correctly)
		var unpacked map[string]interface{}
		decoder := NewMessagePackDecoder(rawBytes)
		val, decErr := decoder.DecodeValue()
		if decErr == nil {
			if m, ok := val.(map[string]interface{}); ok {
				unpacked = m
			}
		}

		// Fallback: vmihailenco/msgpack
		if unpacked == nil {
			if msgpack.Unmarshal(rawBytes, &unpacked) != nil {
				unpacked = nil
			}
		}

		// Last resort: JSON decode
		if unpacked == nil {
			if json.Unmarshal(rawBytes, &unpacked) != nil {
				log.Printf("[decodeSPP] all decoders failed for biz=40 obj=%d, customErr=%v", int(obj), decErr)
				return nil
			}
		}

		// Try standard msgpack numeric-key structure: unpacked["1"] is the message
		msg := getMapVal(unpacked, "1")

		// For obj=40006 and similar: try JSON-style keys (operation, content, etc.)
		if msg == nil {
			// Try to extract from JSON-style operation structure
			op := getMapVal(unpacked, "operation")
			if op == nil {
				op = unpacked // use root as fallback
			}
			contentObj := getMapVal(op, "content")
			if contentObj != nil {
				ct, _ := contentObj["contentType"].(float64)
				if int(ct) == 1 {
					textMap := getMapVal(contentObj, "text")
					text, _ := textMap["text"].(string)
					senderUID, _ := op["senderUid"].(string)
					if senderUID == "" {
						senderUID, _ = unpacked["senderUid"].(string)
					}
					if senderUID != "" && text != "" {
						senderID := strings.Split(senderUID, "@")[0]
						return map[string]interface{}{
							"senderId": senderID, "senderNick": "",
							"content": text, "contentType": int(ct), "gmtCreate": 0,
						}
					}
				}
			}

			// If still nothing, try to find any senderId + content at root level
			if sid, ok := unpacked["senderId"].(string); ok && sid != "" {
				content, _ := unpacked["content"].(string)
				return map[string]interface{}{
					"senderId": sid, "senderNick": "",
					"content": content, "contentType": 0, "gmtCreate": 0,
				}
			}

			log.Printf("[decodeSPP] biz=40 obj=%d: no message structure found, keys=%v",
				int(obj), mapKeys(unpacked))
			return nil
		}

		// Standard msgpack numeric-key path (obj=40000 and compatible)
		senderWrap := getMapVal(msg, "1")
		senderUIDRaw := ""
		if senderWrap != nil {
			senderUIDRaw = getStrFromMap(senderWrap, "1")
		} else if s, ok := msg["1"].(string); ok {
			senderUIDRaw = s
		}

		ext := getMapVal(msg, "10")
		if ext == nil {
			ext = make(map[string]interface{})
		}
		senderID, _ := ext["senderUserId"].(string)
		if senderID == "" && senderUIDRaw != "" {
			senderID = strings.Split(senderUIDRaw, "@")[0]
		}
		senderNick, _ := ext["reminderTitle"].(string)

		contentWrap := getMapVal(msg, "6")
		var inner map[string]interface{}
		if contentWrap != nil {
			inner = getMapVal(contentWrap, "3")
		}
		if inner == nil {
			inner = make(map[string]interface{})
		}

		textPreview := getStrFromMap(inner, "2")
		contentType := inner["4"]
		fullJSONStr := getStrFromMap(inner, "5")

		content := textPreview
		if fullJSONStr != "" {
			var cj map[string]interface{}
			if json.Unmarshal([]byte(fullJSONStr), &cj) == nil {
				if ct, ok := cj["contentType"].(float64); ok && int(ct) == 1 {
					if textMap, ok := cj["text"].(map[string]interface{}); ok {
						if t, ok := textMap["text"].(string); ok {
							content = t
						}
					}
				}
			}
		}

		gmtCreate := msg["5"]
		if senderID == "" {
			return nil
		}
		return map[string]interface{}{
			"senderId": senderID, "senderNick": senderNick,
			"content": content, "contentType": contentType, "gmtCreate": gmtCreate,
		}
	}

	if int(biz) == 370 {
		rawBytes, err := base64.StdEncoding.DecodeString(rawData)
		if err != nil {
			return nil
		}
		var parsed map[string]interface{}
		if json.Unmarshal(rawBytes, &parsed) != nil {
			return nil
		}
		op, _ := parsed["operation"].(map[string]interface{})
		contentObj, _ := op["content"].(map[string]interface{})
		ct, _ := contentObj["contentType"].(float64)
		if int(ct) == 1 {
			textMap, _ := contentObj["text"].(map[string]interface{})
			text, _ := textMap["text"].(string)
			sender, _ := op["senderUid"].(string)
			return map[string]interface{}{
				"senderId": sender, "senderNick": "", "content": text,
				"contentType": int(ct), "gmtCreate": 0,
			}
		}
	}
	return nil
}

func truncStr(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}

func (ws *GoofishWebSocket) parsePushMessages(raw []byte) []map[string]interface{} {
	var data map[string]interface{}
	if json.Unmarshal(raw, &data) != nil {
		return nil
	}

	lwp, _ := data["lwp"].(string)
	log.Printf("[parsePush] lwp=%s, rawLen=%d", lwp, len(raw))

	isSync := lwp == "/s/sync" || strings.Contains(lwp, "syncPushPackage") ||
		strings.Contains(lwp, "vulcan") || strings.Contains(strings.ToLower(lwp), "sync") ||
		lwp == "/s/para"

	if isSync {
		headers, _ := data["headers"].(map[string]interface{})
		mid, _ := headers["mid"].(string)
		sid, _ := headers["sid"].(string)
		if mid != "" {
			go ws.ackMessage(mid, sid)
		}
	}

	body, ok := data["body"].(map[string]interface{})
	if !ok {
		if isSync || lwp == "/s/para" {
			log.Printf("[parsePush] body is not map, type=%T, lwp=%s", data["body"], lwp)
		}
		return nil
	}

	var results []map[string]interface{}

	spp, _ := body["syncPushPackage"].(map[string]interface{})
	if spp != nil {
		items, _ := spp["data"].([]interface{})
		log.Printf("[parsePush] syncPushPackage found, %d items", len(items))
		for idx, item := range items {
			if m, ok := item.(map[string]interface{}); ok {
				bizType := m["bizType"]
				objType := m["objectType"]
				log.Printf("[parsePush] item[%d] biz=%v obj=%v dataLen=%d", idx, bizType, objType, len(fmt.Sprint(m["data"])))
				if parsed := ws.decodeSPPItem(m); parsed != nil {
					results = append(results, parsed)
				} else {
					log.Printf("[parsePush] item[%d] decodeSPPItem returned nil", idx)
				}
			}
		}
	} else if isSync || lwp == "/s/para" {
		// Log body keys for sync/para messages without syncPushPackage
		keys := make([]string, 0, len(body))
		for k := range body {
			keys = append(keys, k)
		}
		log.Printf("[parsePush] no syncPushPackage, body keys=%v", keys)
	}

	if len(results) == 0 {
		pushData, _ := body["data"].(string)
		if pushData != "" {
			decoded := DecryptMessage(pushData)
			if m, ok := decoded.(map[string]interface{}); ok {
				if _, hasSender := m["senderId"]; hasSender {
					results = append(results, m)
				}
			}
		}
	}
	return results
}

func (ws *GoofishWebSocket) ackMessage(mid, sid string) {
	ack := map[string]interface{}{
		"code":    200,
		"headers": map[string]interface{}{"mid": mid, "sid": sid},
	}
	ws.writeJSON(ack)
}

// Close closes the WebSocket connection and stops all goroutines.
func (ws *GoofishWebSocket) Close() {
	ws.running = false
	select {
	case ws.stopCh <- struct{}{}:
	default:
	}
	if ws.ws != nil {
		ws.ws.Close()
		ws.ws = nil
	}
}

func mapKeys(m map[string]interface{}) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}

func getMapVal(m map[string]interface{}, key string) map[string]interface{} {
	if v, ok := m[key].(map[string]interface{}); ok {
		return v
	}
	return nil
}

// extractCID tries to find a conversation ID from a WebSocket response.
// The server may return the CID in various formats:
//   - body.singleChatConversation.cid
//   - body.cid (flat conversation object)
//   - body[0].cid (array form)
//   - body[0].singleChatConversation.cid
func extractCID(data map[string]interface{}) string {
	if body, ok := data["body"].(map[string]interface{}); ok {
		if cid := findCIDInMap(body); cid != "" {
			return cid
		}
	}
	if bodyArr, ok := data["body"].([]interface{}); ok {
		for _, item := range bodyArr {
			if m, ok := item.(map[string]interface{}); ok {
				if cid := findCIDInMap(m); cid != "" {
					return cid
				}
			}
		}
	}
	return ""
}

// findCIDInMap searches for a cid field in a map, including nested singleChatConversation.
func findCIDInMap(m map[string]interface{}) string {
	// Direct cid field
	if cid, ok := m["cid"].(string); ok && cid != "" {
		return strings.Split(cid, "@")[0]
	}
	// Nested in singleChatConversation
	if conv, ok := m["singleChatConversation"].(map[string]interface{}); ok {
		if cid, ok := conv["cid"].(string); ok && cid != "" {
			return strings.Split(cid, "@")[0]
		}
	}
	// Nested in conversation
	if conv, ok := m["conversation"].(map[string]interface{}); ok {
		if cid, ok := conv["cid"].(string); ok && cid != "" {
			return strings.Split(cid, "@")[0]
		}
	}
	return ""
}

// respCode extracts the response code — DingTalk puts it at root OR in headers.
func respCode(data map[string]interface{}) string {
	if c, ok := data["code"]; ok && c != nil {
		if f, ok := c.(float64); ok {
			return fmt.Sprintf("%.0f", f)
		}
		return fmt.Sprint(c)
	}
	if h, ok := data["headers"].(map[string]interface{}); ok {
		if c, ok := h["code"]; ok && c != nil {
			if f, ok := c.(float64); ok {
				return fmt.Sprintf("%.0f", f)
			}
			return fmt.Sprint(c)
		}
	}
	return ""
}


func getStrFromMap(m map[string]interface{}, key string) string {
	if v, ok := m[key].(string); ok {
		return v
	}
	if v, ok := m[key]; ok && v != nil {
		if f, fok := v.(float64); fok && f == float64(int64(f)) {
			return fmt.Sprintf("%.0f", f)
		}
		return fmt.Sprint(v)
	}
	return ""
}

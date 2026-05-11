package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func newChatModel() Model {
	m := New("http://localhost:8080")
	m.currentView = viewChat
	m.token = "tok"
	m.userID = "u1"
	m.username = "alice"
	m.width, m.height = 100, 40
	m.chatPrompting = false
	m.chatInput = newChatInput()
	m.chatViewport = newChatViewport(100, 35)
	return m
}

func TestChatMessageAppended(t *testing.T) {
	m := newChatModel()
	next, cmd := m.Update(wsMsgReceived{userID: "u2", username: "bob", text: "hey!"})
	m2 := next.(Model)
	assert.Len(t, m2.chatMessages, 1)
	assert.Equal(t, "hey!", m2.chatMessages[0].text)
	assert.False(t, m2.chatMessages[0].isSystem)
	assert.NotNil(t, cmd) // re-subscribes to WS
}

func TestChatJoinAppendsSystemMsg(t *testing.T) {
	m := newChatModel()
	next, _ := m.Update(wsJoined{username: "carol"})
	m2 := next.(Model)
	assert.Len(t, m2.chatMessages, 1)
	assert.True(t, m2.chatMessages[0].isSystem)
	assert.Contains(t, m2.chatMessages[0].text, "carol")
}

func TestChatLeftAppendsSystemMsg(t *testing.T) {
	m := newChatModel()
	next, _ := m.Update(wsLeft{username: "carol"})
	m2 := next.(Model)
	assert.Len(t, m2.chatMessages, 1)
	assert.True(t, m2.chatMessages[0].isSystem)
}

func TestWsErrMsgAppendsSystemMsg(t *testing.T) {
	m := newChatModel() // chatConn is nil
	next, cmd := m.Update(wsErrMsg{text: "message too long"})
	m2 := next.(Model)
	assert.Len(t, m2.chatMessages, 1)
	assert.True(t, m2.chatMessages[0].isSystem)
	assert.Contains(t, m2.chatMessages[0].text, "message too long")
	assert.Nil(t, cmd) // no chatConn → no waitForWS re-issued
}

func TestChatInputCharLimitSet(t *testing.T) {
	inp := newChatInput()
	assert.Equal(t, chatMaxMsgLen, inp.CharLimit)
}

func TestFormatChatMsgSelf(t *testing.T) {
	msg := chatMessage{userID: "u1", username: "alice", text: "hello"}
	rendered := formatChatMsg(msg, "u1")
	assert.Contains(t, rendered, "You")
	assert.Contains(t, rendered, "hello")
}

func TestFormatChatMsgOther(t *testing.T) {
	msg := chatMessage{userID: "u2", username: "bob", text: "world"}
	rendered := formatChatMsg(msg, "u1")
	assert.Contains(t, rendered, "bob")
	assert.Contains(t, rendered, "world")
}

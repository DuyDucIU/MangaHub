package main

import (
	"encoding/base64"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
)

func makeTestToken(userID, username string) string {
	payload, _ := json.Marshal(map[string]string{"user_id": userID, "username": username})
	enc := base64.RawURLEncoding.EncodeToString(payload)
	return "header." + enc + ".sig"
}

func TestLoginSuccessSetsState(t *testing.T) {
	m := New("http://localhost:8080")
	m.currentView = viewLogin
	m.authInputs = initLoginInputs()

	tok := makeTestToken("u1", "alice")
	next, cmds := m.Update(loginSuccessMsg{token: tok, userID: "u1", username: "alice"})
	m2 := next.(Model)

	assert.Equal(t, "alice", m2.username)
	assert.Equal(t, "u1", m2.userID)
	assert.Equal(t, tok, m2.token)
	assert.Equal(t, viewMenu, m2.currentView)
	assert.NotNil(t, cmds) // should fire cmdConnectTCP + cmdConnectUDP
}

func TestLoginErrSetsAuthErr(t *testing.T) {
	m := New("http://localhost:8080")
	m.currentView = viewLogin
	m.authInputs = initLoginInputs()

	next, _ := m.Update(errMsg{text: "invalid credentials"})
	m2 := next.(Model)
	assert.Equal(t, "invalid credentials", m2.authErr)
	assert.Equal(t, viewLogin, m2.currentView)
}

func TestRegisterSuccessSwitchesToLogin(t *testing.T) {
	m := New("http://localhost:8080")
	m.currentView = viewRegister
	m.authInputs = initRegisterInputs()

	next, _ := m.Update(registerSuccessMsg{})
	m2 := next.(Model)
	assert.Equal(t, viewLogin, m2.currentView)
	assert.Len(t, m2.authInputs, 2) // re-initialised as login form
}

func TestParseJWTClaims(t *testing.T) {
	tok := makeTestToken("u42", "bob")
	uid, uname, ok := parseJWTClaims(tok)
	assert.True(t, ok)
	assert.Equal(t, "u42", uid)
	assert.Equal(t, "bob", uname)
}

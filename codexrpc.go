// Copyright 2026 Triple Down AB
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"sync"
)

// ---- JSON-RPC framing for `codex app-server` ----
//
// One JSON object per line, in both directions, but the traffic is three
// different things and telling them apart is the whole job of this file:
//
//	{"id":1,"result":{…}}                        a reply to something we asked
//	{"method":"item/started","params":{…}}       a notification, no reply wanted
//	{"method":"…/requestApproval","id":0,…}      a request TO us, reply required
//
// Two shapes on the wire decide the code below. The server does not echo
// `jsonrpc` on anything it sends, so presence of that field cannot classify a
// frame. And server request ids start at **0**, so an `int` field cannot tell
// "no id" from "id zero" — every id here is a pointer for that reason. Getting
// it wrong turns the first approval request of every session into a response
// nobody is waiting for, and the turn hangs.

// codexFrame is one decoded line. Which fields are populated says what it is;
// see classify.
type codexFrame struct {
	ID     *int64          `json:"id"`
	Method string          `json:"method"`
	Params json.RawMessage `json:"params"`
	Result json.RawMessage `json:"result"`
	Error  *codexRPCError  `json:"error"`
}

// codexRPCError is the error member of a failed response.
type codexRPCError struct {
	Code    int             `json:"code"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data"`
}

func (e *codexRPCError) Error() string {
	if e == nil {
		return ""
	}
	return fmt.Sprintf("codex rpc error %d: %s", e.Code, e.Message)
}

// frameKind is what one inbound line turned out to be.
type frameKind int

const (
	frameResponse     frameKind = iota // a reply to one of our calls
	frameServerReq                     // a request to us; we must answer it
	frameNotification                  // fire-and-forget
)

// classify sorts an inbound frame. The id pointer is load-bearing: a server
// request carries a method AND an id, a notification carries a method and no
// id, and a response carries an id and no method.
func (f codexFrame) classify() frameKind {
	switch {
	case f.Method == "":
		return frameResponse
	case f.ID != nil:
		return frameServerReq
	default:
		return frameNotification
	}
}

// codexPending tracks the calls waiting for a reply, keyed by the id we sent.
//
// Separate from any id the server chooses for its own requests: the two id
// spaces are independent, and only direction tells them apart.
type codexPending struct {
	mu   sync.Mutex
	next int64
	wait map[int64]chan codexFrame
}

func newCodexPending() *codexPending {
	// Start at 1. Nothing depends on it, but leaving 0 unused keeps our ids
	// visually distinct from the server's in a -debug log, which is the first
	// place anyone looks when a reply goes missing.
	return &codexPending{next: 1, wait: map[int64]chan codexFrame{}}
}

// begin reserves an id and the channel its reply will arrive on.
func (p *codexPending) begin() (int64, chan codexFrame) {
	p.mu.Lock()
	defer p.mu.Unlock()
	id := p.next
	p.next++
	ch := make(chan codexFrame, 1)
	p.wait[id] = ch
	return id, ch
}

// deliver hands a reply to whoever is waiting for it. An id nobody is waiting
// for is dropped: it means a call already gave up, and there is no one to tell.
func (p *codexPending) deliver(f codexFrame) {
	if f.ID == nil {
		return
	}
	p.mu.Lock()
	ch, ok := p.wait[*f.ID]
	delete(p.wait, *f.ID)
	p.mu.Unlock()
	if ok {
		ch <- f
	}
}

// abandon releases a waiter without a reply, so a caller that timed out or a
// subprocess that died does not leak an entry.
func (p *codexPending) abandon(id int64) {
	p.mu.Lock()
	delete(p.wait, id)
	p.mu.Unlock()
}

// failAll wakes every waiter with the same error. Called when the subprocess
// exits: without it, each in-flight call blocks until its own timeout, and the
// UI freezes for as long as the longest one.
func (p *codexPending) failAll(err error) {
	p.mu.Lock()
	waiters := p.wait
	p.wait = map[int64]chan codexFrame{}
	p.mu.Unlock()
	msg := err.Error()
	for _, ch := range waiters {
		ch <- codexFrame{Error: &codexRPCError{Message: msg}}
	}
}

// errEngineClosed is what an in-flight call sees when the subprocess exits.
var errEngineClosed = errors.New("codex app-server exited")

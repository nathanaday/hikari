// Package api holds the wire types of the hub's node API and a client for it.
package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"time"
)

type Message struct {
	ID   int64     `json:"id"`
	From string    `json:"from"`
	To   string    `json:"to"`
	Body string    `json:"body"`
	Time time.Time `json:"time"`
}

type Peer struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
}

type Self struct {
	Name         string `json:"name"`
	Description  string `json:"description,omitempty"`
	Instructions string `json:"instructions,omitempty"`
	Model        string `json:"model,omitempty"`
	Neighbors    []Peer `json:"neighbors"`
}

type SendRequest struct {
	To   string `json:"to"`
	Body string `json:"body"`
}

const (
	StateStarting = "starting"
	StateIdle     = "idle"
	StateWorking  = "working"
	StateError    = "error"
	StateExited   = "exited"
)

type Status struct {
	State  string `json:"state"`
	Detail string `json:"detail,omitempty"`
}

type errorBody struct {
	Error string `json:"error"`
}

type Client struct {
	URL   string
	Token string
	HTTP  *http.Client
}

func ClientFromEnv() (*Client, error) {
	c := &Client{
		URL:   os.Getenv("HIKARI_HUB_URL"),
		Token: os.Getenv("HIKARI_TOKEN"),
		HTTP:  &http.Client{Timeout: 90 * time.Second},
	}
	if c.URL == "" || c.Token == "" {
		return nil, errors.New("HIKARI_HUB_URL and HIKARI_TOKEN must be set")
	}
	return c, nil
}

func (c *Client) Self(ctx context.Context) (Self, error) {
	var s Self
	err := c.do(ctx, http.MethodGet, "/api/node/self", nil, &s)
	return s, err
}

func (c *Client) Send(ctx context.Context, to, body string) (Message, error) {
	var m Message
	err := c.do(ctx, http.MethodPost, "/api/node/send", SendRequest{To: to, Body: body}, &m)
	return m, err
}

// Inbox removes and returns the node's pending messages. If none are pending,
// it waits up to wait for one to arrive.
func (c *Client) Inbox(ctx context.Context, wait time.Duration) ([]Message, error) {
	var msgs []Message
	path := "/api/node/inbox?wait=" + url.QueryEscape(wait.String())
	err := c.do(ctx, http.MethodGet, path, nil, &msgs)
	return msgs, err
}

func (c *Client) SetStatus(ctx context.Context, s Status) error {
	return c.do(ctx, http.MethodPut, "/api/node/status", s, nil)
}

func (c *Client) do(ctx context.Context, method, path string, in, out any) error {
	var body io.Reader
	if in != nil {
		b, err := json.Marshal(in)
		if err != nil {
			return err
		}
		body = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.URL+path, body)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.Token)
	if in != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		var e errorBody
		if json.NewDecoder(resp.Body).Decode(&e) == nil && e.Error != "" {
			return errors.New(e.Error)
		}
		return fmt.Errorf("hub returned %s", resp.Status)
	}
	if out == nil {
		return nil
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

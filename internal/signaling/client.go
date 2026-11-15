package signaling

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const maxResponseBytes = 64 << 10 // 64 KiB

type Client struct {
	baseURL    string
	httpClient *http.Client
}

func NewClient(baseURL string) *Client {
	return &Client{
		baseURL:    strings.TrimRight(baseURL, "/"),
		httpClient: &http.Client{Timeout: 10 * time.Second},
	}
}

func (c *Client) CreateRoom(ctx context.Context, addrs []string) (string, error) {
	var resp CreateRoomResponse
	if err := c.call(ctx, "/rooms", CreateRoomRequest{Addrs: addrs}, &resp); err != nil {
		return "", err
	}
	if resp.Code == "" {
		return "", errors.New("server returned empty code")
	}
	return resp.Code, nil
}

func (c *Client) JoinRoom(ctx context.Context, code string) ([]string, error) {
	code = strings.ToUpper(strings.TrimSpace(code))
	if code == "" {
		return nil, ErrNotFound
	}

	var resp JoinRoomResponse
	if err := c.call(ctx, "/rooms/"+url.PathEscape(code)+"/join", nil, &resp); err != nil {
		return nil, err
	}
	if len(resp.Addrs) == 0 {
		return nil, errors.New("server returned no addresses")
	}
	return resp.Addrs, nil
}

func (c *Client) call(ctx context.Context, path string, body, out any) error {
	var reqBody io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("encoding request: %w", err)
		}
		reqBody = bytes.NewReader(b)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+path, reqBody)
	if err != nil {
		return fmt.Errorf("building request: %w", err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("contacting signaling server: %w", err)
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes+1))
	if err != nil {
		return fmt.Errorf("reading response: %w", err)
	}
	if len(data) > maxResponseBytes {
		return errors.New("signaling server response too large")
	}
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		if err := json.Unmarshal(data, out); err != nil {
			return fmt.Errorf("unexpected response from %s", c.baseURL)
		}
		return nil
	}

	var apiErr errorResponse
	if json.Unmarshal(data, &apiErr) != nil || apiErr.Error == "" {
		return fmt.Errorf("unexpected response (HTTP %d) from %s", resp.StatusCode, c.baseURL)
	}
	if resp.StatusCode == http.StatusNotFound {
		return ErrNotFound
	}
	return fmt.Errorf("signaling server rejected the request (HTTP %d): %s", resp.StatusCode, apiErr.Error)
}

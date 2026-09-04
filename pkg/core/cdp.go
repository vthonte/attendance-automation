package core

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gobwas/ws"
	"github.com/gobwas/ws/wsutil"
)

type cdpTargetInfo struct {
	ID                   string `json:"id"`
	Type                 string `json:"type"`
	Title                string `json:"title"`
	URL                  string `json:"url"`
	WebSocketDebuggerURL string `json:"webSocketDebuggerUrl"`
}

type cdpRequest struct {
	ID     int64  `json:"id"`
	Method string `json:"method"`
	Params any    `json:"params,omitempty"`
}

type cdpResponse struct {
	ID     int64           `json:"id"`
	Result json.RawMessage `json:"result,omitempty"`
	Error  *struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

type CDPClient struct {
	conn       net.Conn
	msgID      int64
	mu         sync.Mutex
	pending    map[int64]chan cdpResponse
	closedChan chan struct{}
	closeOnce  sync.Once
}

func ConnectCDP(ctx context.Context, host string, port int) (*CDPClient, error) {
	// 1. Query /json/list to get targets
	listURL := fmt.Sprintf("http://%s:%d/json/list", host, port)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, listURL, nil)
	if err != nil {
		return nil, err
	}

	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to query CDP targets at %s: %w", listURL, err)
	}
	defer resp.Body.Close()

	var targets []cdpTargetInfo
	if err := json.NewDecoder(resp.Body).Decode(&targets); err != nil {
		return nil, fmt.Errorf("failed to parse CDP targets: %w", err)
	}

	var wsURL string
	for _, t := range targets {
		if t.Type == "page" && t.WebSocketDebuggerURL != "" {
			wsURL = t.WebSocketDebuggerURL
			break
		}
	}

	if wsURL == "" && len(targets) > 0 {
		wsURL = targets[0].WebSocketDebuggerURL
	}

	if wsURL == "" {
		// Fallback to /json/version
		verURL := fmt.Sprintf("http://%s:%d/json/version", host, port)
		vReq, _ := http.NewRequestWithContext(ctx, http.MethodGet, verURL, nil)
		vResp, vErr := client.Do(vReq)
		if vErr == nil {
			defer vResp.Body.Close()
			var vData struct {
				WebSocketDebuggerURL string `json:"webSocketDebuggerUrl"`
			}
			if err := json.NewDecoder(vResp.Body).Decode(&vData); err == nil && vData.WebSocketDebuggerURL != "" {
				wsURL = vData.WebSocketDebuggerURL
			}
		}
	}

	if wsURL == "" {
		return nil, fmt.Errorf("no usable WebSocket debugger URL found on %s:%d", host, port)
	}

	conn, _, _, err := ws.Dial(ctx, wsURL)
	if err != nil {
		return nil, fmt.Errorf("failed to dial CDP websocket %s: %w", wsURL, err)
	}

	cdp := &CDPClient{
		conn:       conn,
		pending:    make(map[int64]chan cdpResponse),
		closedChan: make(chan struct{}),
	}

	go cdp.readLoop()

	return cdp, nil
}

func (c *CDPClient) readLoop() {
	defer c.Close()

	for {
		select {
		case <-c.closedChan:
			return
		default:
		}

		data, err := wsutil.ReadServerText(c.conn)
		if err != nil {
			return
		}

		var resp cdpResponse
		if err := json.Unmarshal(data, &resp); err != nil {
			continue
		}

		if resp.ID != 0 {
			c.mu.Lock()
			ch, ok := c.pending[resp.ID]
			delete(c.pending, resp.ID)
			c.mu.Unlock()

			if ok {
				ch <- resp
			}
		}
	}
}

func (c *CDPClient) Call(ctx context.Context, method string, params any) (json.RawMessage, error) {
	id := atomic.AddInt64(&c.msgID, 1)
	req := cdpRequest{
		ID:     id,
		Method: method,
		Params: params,
	}

	data, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}

	respChan := make(chan cdpResponse, 1)

	c.mu.Lock()
	c.pending[id] = respChan
	c.mu.Unlock()

	defer func() {
		c.mu.Lock()
		delete(c.pending, id)
		c.mu.Unlock()
	}()

	if err := wsutil.WriteClientText(c.conn, data); err != nil {
		return nil, fmt.Errorf("failed to send CDP message: %w", err)
	}

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-c.closedChan:
		return nil, fmt.Errorf("CDP connection closed")
	case resp := <-respChan:
		if resp.Error != nil {
			return nil, fmt.Errorf("CDP error (%d): %s", resp.Error.Code, resp.Error.Message)
		}
		return resp.Result, nil
	}
}

func (c *CDPClient) GrantPermissions(ctx context.Context, origin string, permissions []string) error {
	cleanOrigin := origin
	if u, err := url.Parse(origin); err == nil && u.Scheme != "" && u.Host != "" {
		cleanOrigin = fmt.Sprintf("%s://%s", u.Scheme, u.Host)
	}

	params := map[string]any{
		"origin":      cleanOrigin,
		"permissions": permissions,
	}
	_, err := c.Call(ctx, "Browser.grantPermissions", params)
	return err
}

func (c *CDPClient) ClearGeolocationOverride(ctx context.Context) error {
	_, err := c.Call(ctx, "Emulation.clearGeolocationOverride", map[string]any{})
	return err
}

func (c *CDPClient) Navigate(ctx context.Context, url string) error {
	params := map[string]any{
		"url": url,
	}
	_, err := c.Call(ctx, "Page.navigate", params)
	return err
}

func (c *CDPClient) Evaluate(ctx context.Context, expression string) (json.RawMessage, error) {
	params := map[string]any{
		"expression":    expression,
		"returnByValue": true,
		"awaitPromise":  true,
	}
	res, err := c.Call(ctx, "Runtime.evaluate", params)
	if err != nil {
		return nil, err
	}

	var evalResult struct {
		Result struct {
			Type  string          `json:"type"`
			Value json.RawMessage `json:"value"`
		} `json:"result"`
		ExceptionDetails *struct {
			Text string `json:"text"`
		} `json:"exceptionDetails"`
	}

	if err := json.Unmarshal(res, &evalResult); err != nil {
		return nil, err
	}

	if evalResult.ExceptionDetails != nil {
		return nil, fmt.Errorf("javascript exception: %s", evalResult.ExceptionDetails.Text)
	}

	return evalResult.Result.Value, nil
}

func (c *CDPClient) Close() {
	c.closeOnce.Do(func() {
		close(c.closedChan)
		if c.conn != nil {
			_ = c.conn.Close()
		}
	})
}

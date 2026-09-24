package goclient

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const unavailable = "Grasshopper unavailable; persistence not acknowledged. Do not automatically retry writes."

type Remote struct {
	endpoint string
	token    string
	client   *http.Client
	protocol string
	session  string
}

func NewRemote(config Config) (*Remote, error) {
	endpoint, err := config.endpoint()
	if err != nil {
		return nil, err
	}
	token, err := config.credential()
	if err != nil {
		return nil, err
	}
	return &Remote{
		endpoint: endpoint.String(), token: token, protocol: "2025-03-26",
		client: &http.Client{Timeout: 5 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }},
	}, nil
}

func rpcID(body []byte) (json.RawMessage, string, string, string, error) {
	var request struct {
		ID     json.RawMessage `json:"id"`
		Method string          `json:"method"`
		Params struct {
			Name string `json:"name"`
			Meta struct {
				Version string `json:"io.modelcontextprotocol/protocolVersion"`
			} `json:"_meta"`
		} `json:"params"`
	}
	if err := json.Unmarshal(body, &request); err != nil {
		return nil, "", "", "", err
	}
	return request.ID, request.Method, request.Params.Name, request.Params.Meta.Version, nil
}

func (r *Remote) Request(ctx context.Context, body []byte) ([]byte, bool, error) {
	if len(body) > 131072 {
		return nil, false, errors.New("MCP input exceeds limit")
	}
	id, method, name, version, err := rpcID(body)
	if err != nil {
		return nil, false, err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, r.endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, false, err
	}
	if version == "" {
		version = r.protocol
	}
	request.Header.Set("Authorization", "Bearer "+r.token)
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept", "application/json, text/event-stream")
	request.Header.Set("MCP-Protocol-Version", version)
	if method != "" {
		request.Header.Set("Mcp-Method", method)
	}
	if name != "" {
		request.Header.Set("Mcp-Name", name)
	}
	if r.session != "" {
		request.Header.Set("Mcp-Session-Id", r.session)
	}
	response, err := r.client.Do(request)
	if err != nil {
		return nil, false, errors.New("backend unavailable")
	}
	defer response.Body.Close()
	if response.StatusCode == http.StatusAccepted {
		return nil, false, nil
	}
	if response.StatusCode != http.StatusOK {
		return nil, false, fmt.Errorf("backend rejected request (%d)", response.StatusCode)
	}
	if session := response.Header.Get("Mcp-Session-Id"); session != "" {
		r.session = session
	}
	var reply []byte
	if strings.HasPrefix(response.Header.Get("Content-Type"), "text/event-stream") {
		reply, err = matchingSSE(response.Body, id)
	} else {
		reply, err = io.ReadAll(io.LimitReader(response.Body, 262145))
		if err == nil && len(reply) > 262144 {
			err = errors.New("backend response exceeds limit")
		}
	}
	if err != nil {
		return nil, false, err
	}
	var envelope struct {
		ID     json.RawMessage `json:"id"`
		Result struct {
			ProtocolVersion string `json:"protocolVersion"`
		} `json:"result"`
	}
	if err := json.Unmarshal(reply, &envelope); err != nil {
		return nil, false, err
	}
	if id != nil && !bytes.Equal(bytes.TrimSpace(envelope.ID), bytes.TrimSpace(id)) {
		return nil, false, errors.New("unexpected response ID")
	}
	if method == "initialize" && envelope.Result.ProtocolVersion != "" {
		r.protocol = envelope.Result.ProtocolVersion
	}
	return reply, true, nil
}

func matchingSSE(stream io.Reader, id json.RawMessage) ([]byte, error) {
	scanner := bufio.NewScanner(io.LimitReader(stream, 262145))
	scanner.Buffer(make([]byte, 4096), 262145)
	var data strings.Builder
	check := func() ([]byte, bool) {
		payload := strings.TrimSpace(data.String())
		data.Reset()
		if payload == "" {
			return nil, false
		}
		var reply struct {
			ID json.RawMessage `json:"id"`
		}
		if json.Unmarshal([]byte(payload), &reply) != nil {
			return nil, false
		}
		return []byte(payload), id == nil || bytes.Equal(bytes.TrimSpace(reply.ID), bytes.TrimSpace(id))
	}
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			if payload, found := check(); found {
				return payload, nil
			}
		} else if value, ok := strings.CutPrefix(line, "data:"); ok {
			data.WriteString(strings.TrimSpace(value))
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	if payload, found := check(); found {
		return payload, nil
	}
	return nil, errors.New("backend did not acknowledge request")
}

func (r *Remote) Context(ctx context.Context, scope Scope, budget int) (json.RawMessage, error) {
	initialize, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": 1, "method": "initialize", "params": map[string]any{"protocolVersion": "2025-03-26", "capabilities": map[string]any{}, "clientInfo": map[string]any{"name": "grasshopper-hook", "version": "1.0"}}})
	answer, acknowledged, err := r.Request(ctx, initialize)
	if err != nil || !acknowledged {
		return nil, errors.New("initialization not acknowledged")
	}
	var initReply struct {
		Error json.RawMessage `json:"error"`
	}
	if json.Unmarshal(answer, &initReply) != nil || initReply.Error != nil {
		return nil, errors.New("initialization rejected")
	}
	_, _, err = r.Request(ctx, []byte(`{"jsonrpc":"2.0","method":"notifications/initialized"}`))
	if err != nil {
		return nil, err
	}
	call, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": 2, "method": "tools/call", "params": map[string]any{"name": "context", "arguments": map[string]any{"scope": scope, "budget": budget}}})
	answer, acknowledged, err = r.Request(ctx, call)
	if err != nil || !acknowledged {
		return nil, errors.New("context not acknowledged")
	}
	var reply struct {
		Error  json.RawMessage `json:"error"`
		Result struct {
			IsError           bool            `json:"isError"`
			StructuredContent json.RawMessage `json:"structuredContent"`
		} `json:"result"`
	}
	if json.Unmarshal(answer, &reply) != nil || reply.Error != nil || reply.Result.IsError || len(reply.Result.StructuredContent) == 0 {
		return nil, errors.New("context rejected")
	}
	return reply.Result.StructuredContent, nil
}

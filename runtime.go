package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

type hostEnvelope struct {
	Type      string `json:"type"`
	RequestID string `json:"requestId"`
	PageID    string `json:"pageId"`
}

type hostConnection struct {
	conn    *websocket.Conn
	writeMu sync.Mutex
}

func runHost(ctx context.Context, config HostConfig, logger *logWriter) error {
	for {
		manifest, pageLookup, summary, err := buildManifest(config)
		if err != nil {
			return err
		}

		logger.Printf(
			"Prepared library: %d series, %d volumes, %d pages",
			summary.Series,
			summary.Volumes,
			summary.Pages,
		)

		err = runSession(ctx, config, manifest, pageLookup, logger)
		if err == nil || context.Cause(ctx) != nil {
			return err
		}

		logger.Printf("Host error: %v", err)
		logger.Printf("Retrying in %.0fs", config.ReconnectDelay.Seconds())

		timer := time.NewTimer(config.ReconnectDelay)
		select {
		case <-ctx.Done():
			timer.Stop()
			return context.Cause(ctx)
		case <-timer.C:
		}
	}
}

func runSession(
	ctx context.Context,
	config HostConfig,
	manifest RegisterManifestMessage,
	pageLookup map[string]HostedPage,
	logger *logWriter,
) error {
	dialer := websocket.Dialer{}
	conn, _, err := dialer.DialContext(ctx, config.WSURL, nil)
	if err != nil {
		return fmt.Errorf("connect to backend: %w", err)
	}
	defer conn.Close()

	hostConn := &hostConnection{conn: conn}

	if _, greetingBytes, err := conn.ReadMessage(); err != nil {
		return fmt.Errorf("read backend greeting: %w", err)
	} else {
		logger.Printf("Backend: %s", string(greetingBytes))
	}

	if err := hostConn.writeJSON(manifest); err != nil {
		return fmt.Errorf("send register manifest: %w", err)
	}

	if _, responseBytes, err := conn.ReadMessage(); err != nil {
		return fmt.Errorf("read manifest registration response: %w", err)
	} else {
		logger.Printf("Backend: %s", string(responseBytes))
	}

	requestCancels := make(map[string]context.CancelFunc)
	var requestMu sync.Mutex
	var requestWG sync.WaitGroup

	defer func() {
		requestMu.Lock()
		for _, cancel := range requestCancels {
			cancel()
		}
		requestMu.Unlock()
		requestWG.Wait()
	}()

	for {
		messageType, payload, err := conn.ReadMessage()
		if err != nil {
			if websocket.IsCloseError(err, websocket.CloseNormalClosure, websocket.CloseGoingAway) {
				return nil
			}
			return err
		}
		if messageType != websocket.TextMessage {
			continue
		}

		var envelope hostEnvelope
		if err := json.Unmarshal(payload, &envelope); err != nil {
			logger.Printf("Ignoring invalid backend message: %v", err)
			continue
		}

		switch envelope.Type {
		case "page_request":
			if envelope.RequestID == "" || envelope.PageID == "" {
				continue
			}

			requestCtx, cancel := context.WithCancel(ctx)
			requestMu.Lock()
			if previousCancel := requestCancels[envelope.RequestID]; previousCancel != nil {
				previousCancel()
			}
			requestCancels[envelope.RequestID] = cancel
			requestMu.Unlock()

			requestWG.Add(1)
			go func(envelope hostEnvelope) {
				defer requestWG.Done()
				defer func() {
					requestMu.Lock()
					delete(requestCancels, envelope.RequestID)
					requestMu.Unlock()
					cancel()
				}()

				if err := servePageRequest(requestCtx, hostConn, pageLookup, envelope); err != nil && requestCtx.Err() == nil {
					logger.Printf("Request %s failed: %v", envelope.RequestID, err)
				}
			}(envelope)

		case "cancel_page_request":
			if envelope.RequestID == "" {
				continue
			}
			requestMu.Lock()
			cancel := requestCancels[envelope.RequestID]
			delete(requestCancels, envelope.RequestID)
			requestMu.Unlock()
			if cancel != nil {
				cancel()
			}
		}
	}
}

func servePageRequest(
	ctx context.Context,
	hostConn *hostConnection,
	pageLookup map[string]HostedPage,
	envelope hostEnvelope,
) error {
	page, ok := pageLookup[envelope.PageID]
	if !ok {
		return hostConn.writeJSON(PageErrorMessage{
			Type:      "page_error",
			RequestID: envelope.RequestID,
			PageID:    envelope.PageID,
			Error:     fmt.Sprintf("Unknown page id: %s", envelope.PageID),
		})
	}

	if err := ctx.Err(); err != nil {
		return err
	}

	pageBytes, err := os.ReadFile(page.Path)
	if err != nil {
		return hostConn.writeJSON(PageErrorMessage{
			Type:      "page_error",
			RequestID: envelope.RequestID,
			PageID:    envelope.PageID,
			Error:     fmt.Sprintf("Failed to read page bytes: %v", err),
		})
	}

	if err := ctx.Err(); err != nil {
		return err
	}

	headerBytes, err := json.Marshal(PageResponseHeader{
		Type:        "page_response",
		RequestID:   envelope.RequestID,
		PageID:      envelope.PageID,
		ContentType: page.ContentType,
	})
	if err != nil {
		return fmt.Errorf("encode page response header: %w", err)
	}

	payload := make([]byte, 0, len(headerBytes)+2+len(pageBytes))
	payload = append(payload, headerBytes...)
	payload = append(payload, '\n', '\n')
	payload = append(payload, pageBytes...)

	return hostConn.writeBinary(payload)
}

func (c *hostConnection) writeJSON(value any) error {
	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	return c.conn.WriteJSON(value)
}

func (c *hostConnection) writeBinary(payload []byte) error {
	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	return c.conn.WriteMessage(websocket.BinaryMessage, payload)
}

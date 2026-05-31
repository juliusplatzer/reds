package net

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

const smesReconnectDelay = 2 * time.Second

var errSmesClientClosed = errors.New("SMES client closed")

type SmesClient struct {
	url string

	incoming      chan SmesFrame
	errors        chan error
	airportChange chan struct{}
	close         chan struct{}

	startOnce sync.Once
	closeOnce sync.Once

	mu      sync.RWMutex
	airport string
}

func NewSmesClient(url string) *SmesClient {
	return &SmesClient{
		url:           url,
		incoming:      make(chan SmesFrame, 256),
		errors:        make(chan error, 16),
		airportChange: make(chan struct{}, 1),
		close:         make(chan struct{}),
	}
}

func (c *SmesClient) Start() {
	if c == nil {
		return
	}
	c.startOnce.Do(func() {
		go c.run()
	})
}

func (c *SmesClient) Close() {
	if c == nil {
		return
	}
	c.closeOnce.Do(func() {
		close(c.close)
	})
}

func (c *SmesClient) SetAirport(icao string) {
	if c == nil {
		return
	}

	icao = strings.ToUpper(strings.TrimSpace(icao))
	c.mu.Lock()
	c.airport = icao
	c.mu.Unlock()

	select {
	case c.airportChange <- struct{}{}:
	default:
	}
}

func (c *SmesClient) Frames() <-chan SmesFrame {
	if c == nil {
		return nil
	}
	return c.incoming
}

func (c *SmesClient) Errors() <-chan error {
	if c == nil {
		return nil
	}
	return c.errors
}

func (c *SmesClient) run() {
	for {
		select {
		case <-c.close:
			return
		default:
		}

		conn, _, err := websocket.DefaultDialer.Dial(c.url, nil)
		if err != nil {
			c.reportError(fmt.Errorf("connect SMES websocket %s: %w", c.url, err))
			if !c.waitToReconnect() {
				return
			}
			continue
		}

		err = c.serve(conn)
		_ = conn.Close()
		if err != nil && !errors.Is(err, errSmesClientClosed) {
			c.reportError(fmt.Errorf("SMES websocket %s: %w", c.url, err))
		}
		if !c.waitToReconnect() {
			return
		}
	}
}

func (c *SmesClient) serve(conn *websocket.Conn) error {
	if err := c.writeAirport(conn); err != nil {
		return err
	}

	readError := make(chan error, 1)
	go func() {
		readError <- c.readFrames(conn)
	}()

	for {
		select {
		case <-c.close:
			return errSmesClientClosed
		case <-c.airportChange:
			if err := c.writeAirport(conn); err != nil {
				return err
			}
		case err := <-readError:
			return err
		}
	}
}

func (c *SmesClient) readFrames(conn *websocket.Conn) error {
	for {
		_, payload, err := conn.ReadMessage()
		if err != nil {
			return err
		}

		var frame SmesFrame
		if err := json.Unmarshal(payload, &frame); err != nil {
			c.reportError(fmt.Errorf("decode SMES frame: %w", err))
			continue
		}
		if frame.Type == "connected" || frame.Key == "" {
			continue
		}

		select {
		case c.incoming <- frame:
		case <-c.close:
			return errSmesClientClosed
		}
	}
}

func (c *SmesClient) writeAirport(conn *websocket.Conn) error {
	c.mu.RLock()
	airport := c.airport
	c.mu.RUnlock()

	airports := []string{}
	if airport != "" {
		airports = append(airports, airport)
	}
	return conn.WriteJSON(SetAirportsMessage{
		Type:     "setAirports",
		Airports: airports,
	})
}

func (c *SmesClient) reportError(err error) {
	select {
	case c.errors <- err:
	default:
	}
}

func (c *SmesClient) waitToReconnect() bool {
	timer := time.NewTimer(smesReconnectDelay)
	defer timer.Stop()

	select {
	case <-timer.C:
		return true
	case <-c.close:
		return false
	}
}

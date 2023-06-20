package client

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

type CardSwipe struct {
	ID     int    // increments for each log entry
	Name   string // name associated with the CardID
	CardID int
	DoorID string
	Time   time.Time
}

type Client struct {
	Addr    string
	Timeout time.Duration

	mut  sync.Mutex
	conn net.Conn
}

// ListSwipes lists all card swipes going back to a particular swipe ID.
// To travel all the way back to the beginning of the log, set earliestID to 0.
func (c *Client) ListSwipes(ctx context.Context, earliestID int, fn func(*CardSwipe) error) error {
	i := 0
	latestID := 0
	for {
		page, err := c.listSwipePage(ctx, latestID)
		if err != nil {
			return fmt.Errorf("getting page %d: %w", i, err)
		}

		for i, item := range page {
			if i == 0 {
				latestID = item.ID - len(page)
			}
			if item.ID <= earliestID {
				return nil
			}
			if err := fn(item); err != nil {
				return err
			}
		}
		if len(page) < 20 {
			return nil // reached the end
		}
		i++
	}
}

func (c *Client) listSwipePage(ctx context.Context, earliestID int) ([]*CardSwipe, error) {
	form := url.Values{}
	form.Add("PC", strconv.Itoa(earliestID+19))
	form.Add("PE", "0")
	form.Add("PN", "Next")
	req, err := http.NewRequest("POST", "http://"+c.Addr+"/ACT_ID_345", strings.NewReader(form.Encode()))
	if err != nil {
		return nil, err
	}

	resp, err := c.doHTTP(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("unexpected response status: %d with body: %s", resp.StatusCode, body)
	}

	return parseSwipesListPage(resp.Body)
}

// doHTTP implements our own HTTP client that strictly reuses connections even when keepalives
// are not properly signaled by the server. This prevents the access control server from rebooting
// due to frequent connections.
func (c *Client) doHTTP(req *http.Request) (resp *http.Response, err error) {
	c.mut.Lock()
	defer c.mut.Unlock()

	if c.conn == nil {
		log.Printf("establishing new connection to the access control server")
		c.conn, err = net.DialTimeout("tcp", c.Addr, c.Timeout)
		if err != nil {
			return nil, err
		}
	}

	defer c.conn.SetDeadline(time.Time{}) // remove timeout
	c.conn.SetDeadline(time.Now().Add(c.Timeout))

	if err := req.Write(c.conn); err != nil {
		c.conn = nil
		return nil, err
	}

	resp, err = http.ReadResponse(bufio.NewReader(c.conn), req)
	if err != nil {
		c.conn = nil
	}

	return resp, err
}
